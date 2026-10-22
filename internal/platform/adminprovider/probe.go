// Package adminprovider connects a manual probe to protected configured sessions.
// It never constructs a Worker or native downloader.
package adminprovider

import (
	"context"
	"errors"
	"github.com/nathanxiangang-web/panta/internal/adminops"
	"github.com/nathanxiangang-web/panta/internal/platform/bootstrap"
	p115 "github.com/nathanxiangang-web/panta/internal/providers/115"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"time"
)

type session struct{ connection, ref string }
type Probe struct {
	secrets  contracts.SecretResolver
	sessions map[session]bool
}

func New(ctx context.Context, path string) (*Probe, error) {
	secrets, sessions, e := bootstrap.LoadAccountSessions(ctx, path)
	if e != nil {
		return nil, adminops.ErrUnavailable
	}
	p := &Probe{secrets: secrets, sessions: map[session]bool{}}
	for _, s := range sessions {
		if s.ProviderID == "115" {
			p.sessions[session{string(s.ConnectionID), string(s.CredentialRef)}] = true
		}
	}
	return p, nil
}
func (p *Probe) Probe(ctx context.Context, t adminops.ProbeTarget) adminops.Capacity {
	unavailable := adminops.Capacity{Status: "NOT_CONNECTED", Source: "115_GET_INFO", ProbedAt: time.Now().UTC()}
	if p == nil || !p.sessions[session{t.ConnectionID, t.CredentialRef}] {
		return unavailable
	}
	secret, e := p.secrets.ResolveSecret(ctx, contracts.CredentialRef(t.CredentialRef))
	if e != nil {
		if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			unavailable.Status = "UNKNOWN"
		}
		return unavailable
	}
	defer clear(secret)
	return p115.GetAccountInfo(ctx, secret)
}
