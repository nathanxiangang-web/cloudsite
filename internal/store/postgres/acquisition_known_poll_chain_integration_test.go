package postgres

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	p115 "github.com/nathanxiangang-web/panta/internal/providers/115"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

type knownHTTPTransport func(*http.Request) (*http.Response, error)

func (transport knownHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type neverStartDownloader struct {
	contracts.DownloaderProvider
	starts int
}

func (provider *neverStartDownloader) StartDownload(context.Context, contracts.DownloadRequest) (contracts.TaskReference, error) {
	provider.starts++
	panic("known polling must never start")
}

type knownPollSession struct{ provider contracts.DownloaderProvider }

func (session knownPollSession) ResolveDownloader(context.Context, acquisition.DownloaderSessionRequest) (contracts.DownloaderBinding, error) {
	return contracts.DownloaderBinding{Descriptor: session.provider.Descriptor(), Downloader: session.provider}, nil
}

func TestPostgresKnownPollRealExecutionAdapterDispatcherChain(t *testing.T) {
	for _, test := range []struct {
		name, body string
		http       int
		timeout    bool
		want       jobs.State
		category   string
	}{
		{"timeout", "", 0, true, jobs.StateRetryWait, "POLL_TRANSPORT_TIMEOUT"},
		{"HTTP503", "PRIVATE-BODY", 503, false, jobs.StateRetryWait, "POLL_HTTP_STATUS"},
		{"decode", "PRIVATE-BODY", 200, false, jobs.StateRecoveryRequired, "POLL_DECODE"},
		{"provider unknown", `{"state":false,"errno":990001}`, 200, false, jobs.StateRecoveryRequired, "POLL_PERMANENT_OR_UNKNOWN"},
		{"running", `{"state":true,"page":1,"page_count":1,"tasks":[{"info_hash":"opaque-ref","status":1}]}`, 200, false, jobs.StateRetryWait, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, pool, outcomeStore, fixture := seedKnownPoll(t, 3)
			if _, err := pool.Exec(ctx, `UPDATE storage_connections SET provider_type='115';UPDATE storage_bindings SET provider_scope='private-test-dir';UPDATE acquisition_provider_tasks SET provider_id='115'`); err != nil {
				t.Fatal(err)
			}
			calls, posts := 0, 0
			originalTransport := http.DefaultTransport
			http.DefaultTransport = knownHTTPTransport(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.Method != http.MethodGet {
					posts++
					t.Fatal("write-method request during poll")
				}
				if test.timeout {
					return nil, &net.DNSError{Err: "PRIVATE", Name: "PRIVATE", IsTimeout: true}
				}
				return &http.Response{StatusCode: test.http, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(test.body)), Request: request}, nil
			})
			defer func() { http.DefaultTransport = originalTransport }()
			adapter, err := p115.NewAdapterFromCookie([]byte("UID=synthetic-uid;CID=synthetic-cid;SEID=synthetic-seid"), p115.Options{HTTPTimeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			provider := &neverStartDownloader{DownloaderProvider: adapter}
			manifestStore, _ := NewAcquisitionManifestRepository(pool)
			jobStore, _ := NewJobRepository(pool)
			taskStore, _ := NewAcquisitionProviderTaskRepository(pool)
			topology, _ := NewStorageRepository(pool)
			manifest, err := manifestStore.GetManifest(ctx, fixture.manifestID)
			if err != nil {
				t.Fatal(err)
			}
			scope := acquisition.KnownPollScope{ManifestID: fixture.manifestID, JobID: fixture.jobID, BindingID: manifest.TargetStorageBindingID, ConnectionID: storage.ConnectionID(manifest.TargetStorageBindingID), ProviderID: "115", RootID: "provider-outcome-root", ReferenceSHA256: acquisition.EvidenceSHA256("opaque-ref"), SourceSHA256: acquisition.EvidenceSHA256(manifest.SourceRef)}
			inputs, _ := acquisition.NewExecutionInputResolver(manifestStore, topology)
			execution, err := acquisition.NewExecutionStepService(inputs, manifestStore, jobStore, knownPollSession{provider}, taskStore, acquisition.WithKnownPollOnly(scope))
			if err != nil {
				t.Fatal(err)
			}
			outcomes, _ := acquisition.NewProviderOutcomeService(outcomeStore)
			dispatcher, err := acquisition.NewStageDispatcher(jobStore, manifestStore, execution, outcomes, &gate311UnexpectedVisibility{}, &gate311UnexpectedCanonical{})
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			result, err := dispatcher.Dispatch(ctx, acquisition.StageDispatchRequest{JobID: fixture.jobID, Owner: fixture.owner, ExpectedClaim: fixture.attempt, Now: now, RetryAt: now.Add(time.Minute), ProjectorPageLimit: 1})
			if err != nil || result.Job.State != test.want || result.DiagnosticCategory != test.category || calls != 1 || posts != 0 || provider.starts != 0 {
				t.Fatalf("result=%#v err=%v calls=%d posts=%d starts=%d", result, err, calls, posts, provider.starts)
			}
			if test.category != "" && (result.Job.LastError == nil || strings.Contains(*result.Job.LastError, "PRIVATE")) {
				t.Fatal("poll error persisted private data")
			}
		})
	}
}
