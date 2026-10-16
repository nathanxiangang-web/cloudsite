package acquisition_test

import (
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
)

const testBTIH = "0123456789abcdef0123456789abcdef01234567"
const testMinimal = "magnet:?xt=urn:btih:" + testBTIH

func TestPrepareMagnetKeepsOnlyBTIHAndDisplayName(t *testing.T) {
	rawBytes, _ := hex.DecodeString(testBTIH)
	b32 := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rawBytes)
	for _, test := range []struct{ input, want string }{
		{testMinimal + "&dn=demo-video&tr=https%3A%2F%2Ftracker.invalid%2Fa&tr=udp%3A%2F%2Ftracker.invalid%3A6969", testMinimal + "&dn=demo-video"},
		{"magnet:?tr=udp%3A%2F%2Ftracker.invalid&dn=demo-video&xt=urn:btih:" + testBTIH, testMinimal + "&dn=demo-video"},
		{"MAGNET:?XT=URN:BTIH:" + strings.ToUpper(testBTIH) + "&Dn=A+B%26C%2B100%25&ws=https%3A%2F%2Fseed.invalid", testMinimal + "&dn=A%20B%26C%2B100%25"},
		{"magnet:?xt=urn:btih:" + strings.ToLower(b32) + "&dn=%E6%B5%8B%E8%AF%95", testMinimal + "&dn=%E6%B5%8B%E8%AF%95"},
		{testMinimal, testMinimal},
		{testMinimal + "&tr=https%3A%2F%2Ftracker.invalid", testMinimal},
		{testMinimal + "&dn=", testMinimal},
		{testMinimal + "&xt=urn:btih:" + strings.ToUpper(testBTIH) + "&dn=same&dn=same", testMinimal + "&dn=same"},
	} {
		got, err := acquisition.PrepareSource(test.input, acquisition.MagnetBTIHNameV1)
		if err != nil || got.Reference.Value != test.want || got.Original != test.input || got.Reference.Scheme != "magnet" || got.Changed != (test.input != test.want) {
			t.Fatalf("preparation mismatch, input bytes=%d error=%v", len(test.input), err)
		}
		again, err := acquisition.PrepareSource(got.Reference.Value, got.Policy)
		if err != nil || again.Reference.Value != got.Reference.Value || again.Changed {
			t.Fatal("not deterministic/idempotent", err)
		}
	}
}
func TestPrepareSourcePreservesLegacyAndNonMagnetBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("source preparation performed HTTP I/O") }))
	defer server.Close()
	for _, input := range []string{server.URL + "/a%2Fb?q=%2f&x=Two#part", "HTTPS://example.invalid/a?q=A+B&token=PRIVATE", "ed2k://|file|A.iso|123|ABC|/"} {
		got, err := acquisition.PrepareSource(input, acquisition.MagnetBTIHNameV1)
		if err != nil || got.Original != input || got.Reference.Value != input || got.Changed {
			t.Fatal("nonmagnet source changed", err)
		}
	}
	original := "magnet:?xt=urn:btih:ABC&dn=Exact+Case&tr=https%3A%2F%2Ftracker.invalid"
	exact, err := acquisition.PrepareSource(original, acquisition.SourceExactV1)
	if err != nil || exact.Reference.Value != original || exact.Changed {
		t.Fatal("legacy opaque policy changed", err)
	}
	legacy, err := acquisition.ResolveSource(original)
	if err != nil || legacy.Value != original {
		t.Fatal("old resolver changed", err)
	}
}
func TestPrepareSourceRejectsAmbiguitySelectionAndMalformedInput(t *testing.T) {
	for _, test := range []struct {
		input string
		want  error
	}{
		{testMinimal + "&xt=urn:btih:" + strings.Repeat("f", 40), acquisition.ErrMagnetAmbiguous},
		{testMinimal + "&dn=one&dn=two", acquisition.ErrMagnetAmbiguous},
		{testMinimal + "&so=0,2-4", acquisition.ErrMagnetUnsupported},
		{testMinimal + "&%73o=", acquisition.ErrMagnetUnsupported},
		{"magnet:?xt=urn:btmh:1220" + strings.Repeat("a", 64), acquisition.ErrMagnetUnsupported},
		{testMinimal + "&xt=urn:btmh:1220" + strings.Repeat("a", 64), acquisition.ErrMagnetUnsupported},
		{"magnet:?xt=urn:btih:short", acquisition.ErrInvalidSource},
		{"magnet:?xt=urn:btih:" + strings.Repeat("z", 40), acquisition.ErrInvalidSource},
		{"magnet:?dn=only-name", acquisition.ErrInvalidSource},
		{testMinimal + "&dn=%0D%0A", acquisition.ErrInvalidSource},
		{testMinimal + "&dn=%00", acquisition.ErrInvalidSource},
		{testMinimal + "&dn=%ff", acquisition.ErrInvalidSource},
		{testMinimal + "&tr=%GG", acquisition.ErrInvalidSource},
		{testMinimal + "#ignored", acquisition.ErrInvalidSource},
		{"magnet://example.invalid/?xt=urn:btih:" + testBTIH, acquisition.ErrInvalidSource},
		{" " + testMinimal, acquisition.ErrInvalidSource},
		{testMinimal + "\n", acquisition.ErrInvalidSource},
		{testMinimal + strings.Repeat("&tr=x", 512), acquisition.ErrInvalidSource},
		{testMinimal + "&dn=" + strings.Repeat("a", 4096), acquisition.ErrInvalidSource},
		{testMinimal + strings.Repeat("x", acquisition.MaxSourceInputLength), acquisition.ErrInvalidSource},
	} {
		got, err := acquisition.PrepareSource(test.input, acquisition.MagnetBTIHNameV1)
		if !errors.Is(err, test.want) || got.Original != "" || got.Reference.Value != "" {
			t.Fatalf("rejection mismatch error=%v expected=%v", err, test.want)
		}
	}
	if _, err := acquisition.PrepareSource(testMinimal, acquisition.SourcePolicy("client-selected")); err != acquisition.ErrSourcePolicy {
		t.Fatal("unknown policy")
	}
}
func TestLargeTrackerInputIsBoundedNotBlindlyTruncatedAndRedacted(t *testing.T) {
	input := testMinimal + "&dn=demo-video" + strings.Repeat("&tr=https%3A%2F%2Ftracker.invalid%2F"+strings.Repeat("x", 40), 100)
	if len(input) <= acquisition.MaxSourceRefLength {
		t.Fatal("fixture not long enough")
	}
	got, err := acquisition.PrepareSource(input, acquisition.MagnetBTIHNameV1)
	if err != nil || got.Reference.Value != testMinimal+"&dn=demo-video" || got.Original != input || got.RemovedParameters != 100 {
		t.Fatal("long magnet was not parsed safely", err)
	}
	if _, err := acquisition.PrepareSource(input, acquisition.SourceExactV1); err == nil {
		t.Fatal("legacy size contract changed")
	}
	encoded, _ := json.Marshal(got)
	for _, text := range []string{string(encoded), fmt.Sprint(got), fmt.Sprintf("%+v", got)} {
		if strings.Contains(text, testBTIH) || strings.Contains(text, "demo-video") || strings.Contains(text, "tracker.invalid") {
			t.Fatal("private source leaked in generic summary")
		}
	}
}
func TestMagnetRejectionPriorityIsDeterministic(t *testing.T) {
	input := testMinimal + "&so=0&dn=one&dn=two"
	for i := 0; i < 100; i++ {
		if _, err := acquisition.PrepareSource(input, acquisition.MagnetBTIHNameV1); err != acquisition.ErrMagnetAmbiguous {
			t.Fatal("unstable error classification", err)
		}
	}
}
func FuzzPrepareMagnetNeverProducesMalformedOrUnboundedOutput(f *testing.F) {
	f.Add(testMinimal + "&dn=demo&tr=udp%3A%2F%2Ftracker.invalid")
	f.Add("magnet:?xt=urn:btih:invalid")
	f.Fuzz(func(t *testing.T, input string) {
		got, err := acquisition.PrepareSource(input, acquisition.MagnetBTIHNameV1)
		if err != nil {
			return
		}
		if got.Original != input || len(got.Reference.Value) > acquisition.MaxSourceRefLength {
			t.Fatal("invalid prepared shape")
		}
		again, err := acquisition.PrepareSource(got.Reference.Value, got.Policy)
		if err != nil || again.Reference.Value != got.Reference.Value || again.Changed {
			t.Fatal("unstable preparation")
		}
	})
}
