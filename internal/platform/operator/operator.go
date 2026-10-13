// Package operator exposes two distinct, explicit process actions over the
// accepted runtime graph. Preflight constructs and closes but never runs it.
package operator

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/app"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/platform/bootstrap"
	"github.com/nathanxiangang-web/panta/internal/platform/config"
	"github.com/nathanxiangang-web/panta/internal/runtime"
)

type Action string

const (
	Preflight Action = "preflight-acquisition"
	Start     Action = "start-acquisition"
)

var ErrInvalidAction = errors.New("invalid acquisition operator action or configuration")
var ErrAlreadyStarting = errors.New("acquisition operator worker already starting or running")
var ErrKnownContinuationStopped = errors.New("known-reference continuation stopped; inspect durable pair")
var startGuard atomic.Bool

// Builder is an integration-test seam. The stock command passes nil and always
// uses the production D-038 constructor. An override cannot originate in CLI
// arguments or the operator JSON file.
type Builder func(context.Context, config.Config, runtime.RuntimeDependencies) (*runtime.Runtime, error)

// Run bounds bootstrap, constructs exactly one graph, and closes it on every
// path. Only Start invokes the worker; Preflight cannot ClaimNext or submit Hint.
func Run(ctx context.Context, action Action, cfg config.Config, bootstrapPath string, builder Builder) error {
	return runScoped(ctx, action, cfg, bootstrapPath, builder, nil)
}

func runScoped(ctx context.Context, action Action, cfg config.Config, bootstrapPath string, builder Builder, scope *acquisition.KnownPollScope) error {
	var stopScoped context.CancelFunc
	var failedScoped atomic.Bool
	if scope != nil {
		ctx, stopScoped = context.WithCancel(ctx)
		defer stopScoped()
	}
	if action != Preflight && action != Start || !cfg.AcquisitionWorker.Enabled ||
		bootstrapPath == "" {
		return ErrInvalidAction
	}
	if err := cfg.Validate(); err != nil {
		return ErrInvalidAction
	}
	if action == Start {
		if !startGuard.CompareAndSwap(false, true) {
			return ErrAlreadyStarting
		}
		defer startGuard.Store(false)
	}
	startupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	deps, err := bootstrap.Load(startupCtx, bootstrapPath)
	if err != nil {
		return bootstrap.Category(err)
	}
	deps.KnownPollScope = scope
	if action == Start {
		deps.WorkerOptions = append(deps.WorkerOptions, app.WithWorkerEventSink(func(event app.WorkerEvent) {
			if scope != nil && knownMustStop(event) {
				failedScoped.Store(event.JobState != jobs.StateSucceeded)
				stopScoped()
			}
			if event.Kind == app.WorkerTransientError ||
				(event.Kind == app.WorkerStage && event.DiagnosticCategory != "") {
				log.Printf("acquisition worker event=%s job_id=%s claim=%d stage=%s error_kind=%s first_error_category=%s elapsed_ms=%d cause_type=%s http_status=%d parser_stage=%s response_shape=%s response_media_family=%s response_size_bucket=%s outer_json_valid=%s poll_handoff=%s",
					event.Kind, event.JobID, event.ClaimAttempts, event.Stage, event.ErrorKind,
					event.DiagnosticCategory, event.DiagnosticElapsedMS, event.DiagnosticCauseType, event.DiagnosticHTTPStatus, event.DiagnosticStage, event.DiagnosticResponseShape,
					event.DiagnosticResponseMediaFamily, event.DiagnosticResponseSizeBucket, event.DiagnosticOuterJSONValid, event.PollHandoff)
			}
		}))
	}
	if builder == nil {
		builder = runtime.NewRuntime
	}
	service, err := builder(startupCtx, cfg, deps)
	if err != nil {
		return err
	}
	if service == nil {
		return ErrInvalidAction
	}
	defer service.Close()
	if action == Preflight {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err = service.Run(ctx)
	if failedScoped.Load() {
		return ErrKnownContinuationStopped
	}
	return err
}

// A committed retry handoff is NOT FIRST_ERROR_STOP. An uncommitted tick error
// or a paired recovery outcome is; SUCCEEDED ends this scoped worker normally.
func knownMustStop(event app.WorkerEvent) bool {
	return event.Kind == app.WorkerTransientError || event.Kind == app.WorkerFatalRecoveryDebt ||
		event.Kind == app.WorkerStage && (event.JobState == jobs.StateRecoveryRequired ||
			event.JobState == jobs.StateSucceeded || event.JobState == jobs.StateFailed || event.JobState == jobs.StateCanceled)
}
