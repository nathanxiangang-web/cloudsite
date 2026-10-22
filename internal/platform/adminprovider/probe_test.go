package adminprovider

import (
	"context"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"testing"
)

func TestMissingOrMismatchedSessionNeverProbes(t *testing.T) {
	for _, p := range []*Probe{nil, {sessions: map[session]bool{{"conn", "ref"}: true}}} {
		c := p.Probe(context.Background(), adminops.ProbeTarget{BindingID: "binding", ConnectionID: "other", CredentialRef: "SECRET"})
		if c.Status != "NOT_CONNECTED" || c.TotalBytes != "" {
			t.Fatal(c)
		}
	}
}
