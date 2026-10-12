package p115

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/SheltonZhu/115driver/pkg/driver"
)

func TestWireUAObservationIsClosedAndLeavesRequestUntouched(t *testing.T) {
	oldWriter := log.Writer()
	defer log.SetOutput(oldWriter)
	for _, test := range []struct{ ua, path, want string }{
		{driver.UA115Browser, "/lixianssp/?ac=add_task_urls", "UA115Browser"},
		{"PRIVATE-HEADER-TOKEN", "/lixianssp/?ac=add_task_urls", "OTHER"},
		{driver.UA115Browser, "/other?secret=PRIVATE-URL", ""},
	} {
		var captured bytes.Buffer
		log.SetOutput(&captured)
		r, _ := http.NewRequest(http.MethodPost, "https://lixian.115.com"+test.path, strings.NewReader("PRIVATE-FORM"))
		r.Header.Set("User-Agent", test.ua)
		r.Header.Set("Cookie", "PRIVATE-COOKIE")
		calls := 0
		next := startRoundTripper(func(sent *http.Request) (*http.Response, error) {
			calls++
			body, _ := io.ReadAll(sent.Body)
			if sent != r || sent.Header.Get("User-Agent") != test.ua || sent.Header.Get("Cookie") != "PRIVATE-COOKIE" || string(body) != "PRIVATE-FORM" {
				t.Fatal("passive observer changed request")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("PRIVATE-RESPONSE"))}, nil
		})
		resp, err := (statusTransport{next: next, trace: &startHTTPTrace{}}).RoundTrip(r)
		if err != nil || calls != 1 {
			t.Fatal("unexpected wire call count")
		}
		resp.Body.Close()
		output := captured.String()
		if strings.Contains(output, "PRIVATE-") || (test.want == "" && output != "") || (test.want != "" && !strings.Contains(output, "offline_user_agent_family="+test.want)) {
			t.Fatal("unclosed observation, missing enum, or secret disclosure")
		}
	}
}
