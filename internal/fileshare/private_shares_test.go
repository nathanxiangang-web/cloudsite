package fileshare

import (
	"context"
	"errors"
	"testing"

	"github.com/nathanxiangang-web/panta/internal/fileaccess"
)

func TestPrivateSharesAreDeniedAtCreateRedeemAndOpen(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{}
	files := &fakeFiles{}
	service, err := New(store, files)
	if err != nil {
		t.Fatal(err)
	}
	target := fileaccess.Request{BindingID: "binding-a", ResourceID: "resource-a", Path: "/云下载/_users/a/result.bin"}
	if _, _, err := service.Create(ctx, owner(), target, "1h", ""); !errors.Is(err, ErrDenied) {
		t.Fatalf("private create error = %v, want ErrDenied", err)
	}
	if store.record.ID != "" {
		t.Fatal("private share was persisted")
	}
	token, err := secret()
	if err != nil {
		t.Fatal(err)
	}
	grantToken, err := secret()
	if err != nil {
		t.Fatal(err)
	}
	store.record = Record{ID: "95000000-0000-4000-8000-000000000009", Owner: owner().UserID,
		Target: target, Name: "result.bin", Size: 4, TokenHash: SecretHash(token)}
	store.grant = SecretHash(grantToken)
	if _, err := service.Redeem(ctx, token, ""); !errors.Is(err, ErrDenied) || store.issued != 0 {
		t.Fatalf("private redeem error = %v, issued = %d", err, store.issued)
	}
	if _, err := service.Open(ctx, store.record.ID, grantToken, "", false); !errors.Is(err, ErrDenied) || files.opened != 0 {
		t.Fatalf("private open error = %v, opened = %d", err, files.opened)
	}
}
