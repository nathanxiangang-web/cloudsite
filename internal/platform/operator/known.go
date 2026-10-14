package operator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/platform/bootstrap"
	"github.com/nathanxiangang-web/panta/internal/platform/config"
	"github.com/nathanxiangang-web/panta/internal/store/postgres"
)

const ResumeKnown Action = "resume-known-acquisition"
const StartKnown Action = "start-known-acquisition"

// This file is an operator-reviewed GO, not a reusable configuration. Default
// Execute=false denies execution. Both actions consume separate durable markers
// before any mutation/worker; crashes and admission failures cannot reset them.
type knownAuthorization struct {
	digest       string
	BinarySHA256 string                           `json:"binary_sha256"`
	Execute      bool                             `json:"execute"`
	ExpiresAt    time.Time                        `json:"expires_at"`
	Request      acquisition.KnownRecoveryRequest `json:"request"`
}

func readKnownAuthorization(path string, now time.Time) (knownAuthorization, error) {
	data, err := bootstrap.ReadProtectedFile(path, 16<<10)
	if err != nil {
		return knownAuthorization{}, ErrInvalidAction
	}
	defer clear(data)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var authorization knownAuthorization
	if decoder.Decode(&authorization) != nil || decoder.Decode(new(any)) != io.EOF || !authorization.Execute ||
		!authorization.Request.Valid() || !authorization.ExpiresAt.After(now) || authorization.ExpiresAt.Sub(now) > 20*time.Minute {
		return knownAuthorization{}, ErrInvalidAction
	}
	binaryDigest, err := hex.DecodeString(authorization.BinarySHA256)
	if err != nil || len(binaryDigest) != sha256.Size || hex.EncodeToString(binaryDigest) != authorization.BinarySHA256 {
		return knownAuthorization{}, ErrInvalidAction
	}
	authorization.digest = acquisition.EvidenceSHA256(string(data))
	return authorization, nil
}

func reserveKnownAction(path string, action Action, digest string) error {
	if action == StartKnown {
		previous, err := bootstrap.ReadProtectedFile(path+"."+string(ResumeKnown)+".reserved", 256)
		if err != nil || string(previous) != "RESERVED "+digest+"\n" {
			return ErrInvalidAction
		}
	}
	marker := path + "." + string(action) + ".reserved"
	file, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrInvalidAction
	}
	defer file.Close()
	if _, err = file.WriteString("RESERVED " + digest + "\n"); err != nil {
		return ErrInvalidAction
	}
	if file.Sync() != nil {
		return ErrInvalidAction
	}
	directory, err := os.Open(filepath.Dir(marker))
	if err != nil {
		return ErrInvalidAction
	}
	defer directory.Close()
	if directory.Sync() != nil {
		return ErrInvalidAction
	}
	return nil
}

// RunKnown never submits intent or starts downloads. An expired GO is rejected
// using PostgreSQL time before reservation. Only StartKnown builds provider
// sessions, with both exact-job claiming and the unconditional no-start fence.
func RunKnown(ctx context.Context, action Action, cfg config.Config, bootstrapPath, authorizationPath string) error {
	if action != ResumeKnown && action != StartKnown || cfg.DatabaseURL == "" || !cfg.AcquisitionWorker.Enabled {
		return ErrInvalidAction
	}
	if cfg.Validate() != nil {
		return ErrInvalidAction
	}
	startupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	pool, err := postgres.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return ErrInvalidAction
	}
	defer pool.Close()
	migrator, err := postgres.NewMigrator(pool)
	if err != nil {
		return ErrInvalidAction
	}
	status, err := migrator.Status(startupCtx)
	if err != nil || !status.Compatible || status.CurrentVersion != postgres.CurrentSchemaVersion || status.LatestVersion != postgres.CurrentSchemaVersion {
		return ErrInvalidAction
	}
	var databaseNow time.Time
	if pool.QueryRow(startupCtx, `SELECT clock_timestamp()`).Scan(&databaseNow) != nil {
		return ErrInvalidAction
	}
	authorization, err := readKnownAuthorization(authorizationPath, databaseNow)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return ErrInvalidAction
	}
	file, err := os.Open(executable)
	if err != nil {
		return ErrInvalidAction
	}
	hash := sha256.New()
	_, hashErr := io.Copy(hash, file)
	closeErr := file.Close()
	if hashErr != nil || closeErr != nil || hex.EncodeToString(hash.Sum(nil)) != authorization.BinarySHA256 {
		return ErrInvalidAction
	}
	if action == StartKnown {
		manifests, _ := postgres.NewAcquisitionManifestRepository(pool)
		jobStore, _ := postgres.NewJobRepository(pool)
		tasks, _ := postgres.NewAcquisitionProviderTaskRepository(pool)
		topology, _ := postgres.NewStorageRepository(pool)
		manifest, manifestErr := manifests.GetManifest(startupCtx, authorization.Request.Scope.ManifestID)
		job, jobErr := jobStore.Get(startupCtx, authorization.Request.Scope.JobID)
		task, taskErr := tasks.GetProviderTask(startupCtx, authorization.Request.Scope.ManifestID)
		binding, bindingErr := topology.GetBinding(startupCtx, authorization.Request.Scope.BindingID)
		if manifestErr != nil || jobErr != nil || taskErr != nil || bindingErr != nil ||
			!authorization.Request.Scope.Matches(manifest, task) || manifest.State != acquisition.StateActive ||
			job.State != jobs.StateRetryWait || job.ClaimAttempts != authorization.Request.ExpectedClaim ||
			acquisition.ValidateLinkedAcquisitionJob(manifest.ID, job) != nil || binding.ConnectionID != authorization.Request.Scope.ConnectionID || binding.IndexCoreRootID != authorization.Request.Scope.RootID {
			return ErrInvalidAction
		}
	}
	if err = reserveKnownAction(authorizationPath, action, authorization.digest); err != nil {
		return err
	}
	boundedCtx, stop := context.WithTimeout(ctx, authorization.ExpiresAt.Sub(databaseNow))
	defer stop()
	if action == ResumeKnown {
		repository, err := postgres.NewKnownRecoveryRepository(pool)
		if err != nil {
			return ErrInvalidAction
		}
		_, err = repository.ResumeKnownAcquisition(boundedCtx, authorization.Request)
		return err
	}
	return runScoped(boundedCtx, Start, cfg, bootstrapPath, nil, &authorization.Request.Scope)
}
