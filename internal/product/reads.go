// Package product owns authenticated application policy and redacted views.
// It is not a Job engine, provider executor or physical-resource authority.
package product

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/catalog"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/jobs"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
)

var (
	ErrInvalid     = errors.New("invalid product request")
	ErrNotFound    = errors.New("product object not found")
	ErrUnavailable = errors.New("product read unavailable")
	ErrAmbiguous   = errors.New("product resource ambiguous")
	idPattern      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

const ReadTimeout = 10 * time.Second

func ValidID(s string) bool { return idPattern.MatchString(s) }
func ValidOpaque(s string) bool {
	return len(s) <= 2048 && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func ValidPage(limit int, cursor string) bool {
	return limit >= 1 && limit <= 50 && (cursor == "" || ValidID(cursor))
}
func missing(port any) bool {
	if port == nil {
		return true
	}
	value := reflect.ValueOf(port)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
}

type TaskRecord struct {
	Manifest acquisition.Manifest
	Job      *jobs.Job
}
type TaskReader interface {
	ListTasks(context.Context, auth.UserID, []storage.BindingID, int, string) ([]TaskRecord, error)
	GetTask(context.Context, auth.UserID, []storage.BindingID, acquisition.ManifestID) (TaskRecord, error)
}
type TopologyReader interface {
	GetBinding(context.Context, storage.BindingID) (storage.Binding, error)
}
type PhysicalReader interface {
	Resolve(context.Context, resourceview.PhysicalRequest) (resourceview.PhysicalDetail, error)
}
type Task struct {
	ManifestID    acquisition.ManifestID `json:"manifest_id"`
	JobID         *jobs.JobID            `json:"job_id"`
	ManifestState acquisition.State      `json:"manifest_state"`
	JobState      *jobs.State            `json:"job_state"`
	ResultCopyID  *catalog.CopyID        `json:"result_copy_id"`
	ClaimAttempts int                    `json:"claim_attempts"`
	FailureCount  int                    `json:"failure_count"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}
type TaskPage struct {
	Items      []Task `json:"items"`
	NextCursor string `json:"next_cursor"`
}
type Resource struct {
	BindingID        storage.BindingID          `json:"binding_id"`
	ResourceID       string                     `json:"resource_id"`
	Path             *string                    `json:"path"`
	Name             *string                    `json:"name"`
	Directory        *bool                      `json:"directory"`
	SizeBytes        *int64                     `json:"size_bytes"`
	Presence         indexcore.ResourcePresence `json:"presence"`
	CopyID           *catalog.CopyID            `json:"copy_id"`
	CopyAvailability *string                    `json:"copy_availability"`
	AssetID          *catalog.AssetID           `json:"asset_id,omitempty"`
	ReleaseID        *catalog.ReleaseID         `json:"release_id,omitempty"`
	VariantID        *catalog.VariantID         `json:"variant_id,omitempty"`
}
type ResourcePage struct {
	Items      []Resource `json:"items"`
	NextCursor string     `json:"next_cursor"`
}
type BrowseRequest struct {
	WholeRoot        bool
	BindingID        storage.BindingID
	ParentResourceID *string
	Cursor           string
	Limit            int
}
type Reads interface {
	Tasks(context.Context, auth.Principal, int, string) (TaskPage, error)
	Task(context.Context, auth.Principal, acquisition.ManifestID) (Task, error)
	Browse(context.Context, auth.Principal, BrowseRequest) (ResourcePage, error)
	Resolve(context.Context, auth.Principal, storage.BindingID, string) (Resource, error)
}
type ReadService struct {
	tasks    TaskReader
	topology TopologyReader
	index    indexcore.ReadPort
	physical PhysicalReader
	copies   catalog.PhysicalIdentityReader
	allowed  []storage.BindingID
	bindings map[storage.BindingID]bool
}

func NewReadService(tasks TaskReader, topology TopologyReader, index indexcore.ReadPort, physical PhysicalReader, copies catalog.PhysicalIdentityReader, allowed []storage.BindingID) (*ReadService, error) {
	if missing(tasks) || missing(topology) || missing(index) || missing(physical) || missing(copies) || len(allowed) == 0 {
		return nil, ErrInvalid
	}
	bindings := make(map[storage.BindingID]bool, len(allowed))
	for _, id := range allowed {
		if !ValidID(string(id)) || bindings[id] {
			return nil, ErrInvalid
		}
		bindings[id] = true
	}
	return &ReadService{tasks: tasks, topology: topology, index: index, physical: physical, copies: copies, allowed: append([]storage.BindingID(nil), allowed...), bindings: bindings}, nil
}
func principalValid(p auth.Principal) error {
	if !p.Valid() {
		return auth.ErrUnauthorized
	}
	return nil
}
func safeReadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return ErrUnavailable
}
func (s *ReadService) Tasks(ctx context.Context, p auth.Principal, limit int, cursor string) (TaskPage, error) {
	if s == nil {
		return TaskPage{}, ErrUnavailable
	}
	if err := principalValid(p); err != nil {
		return TaskPage{}, err
	}
	if !ValidPage(limit, cursor) {
		return TaskPage{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()
	records, err := s.tasks.ListTasks(ctx, p.UserID, s.allowed, limit+1, cursor)
	if err != nil {
		return TaskPage{}, safeReadError(err)
	}
	if len(records) > limit+1 {
		return TaskPage{}, ErrUnavailable
	}
	page := TaskPage{Items: make([]Task, 0, limit)}
	previous := cursor
	for i, record := range records {
		id := string(record.Manifest.ID)
		if id <= previous {
			return TaskPage{}, ErrUnavailable
		}
		previous = id
		item, err := s.taskView(record, p)
		if err != nil {
			return TaskPage{}, err
		}
		if i < limit {
			page.Items = append(page.Items, item)
		} else {
			page.NextCursor = string(page.Items[len(page.Items)-1].ManifestID)
		}
	}
	return page, nil
}
func (s *ReadService) Task(ctx context.Context, p auth.Principal, id acquisition.ManifestID) (Task, error) {
	if s == nil {
		return Task{}, ErrUnavailable
	}
	if err := principalValid(p); err != nil {
		return Task{}, err
	}
	if !ValidID(string(id)) {
		return Task{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()
	record, err := s.tasks.GetTask(ctx, p.UserID, s.allowed, id)
	if err != nil {
		return Task{}, safeReadError(err)
	}
	if record.Manifest.ID != id {
		return Task{}, ErrUnavailable
	}
	return s.taskView(record, p)
}
func (s *ReadService) taskView(record TaskRecord, p auth.Principal) (Task, error) {
	m, j := record.Manifest, record.Job
	if m.UserID == nil || string(*m.UserID) != string(p.UserID) || !s.bindings[m.TargetStorageBindingID] {
		return Task{}, ErrNotFound
	}
	if !ValidID(string(m.ID)) || acquisition.ValidateManifest(m) != nil {
		return Task{}, ErrUnavailable
	}
	dto := Task{ManifestID: m.ID, JobID: m.JobID, ManifestState: m.State, ResultCopyID: m.ResultCopyID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
	if m.State == acquisition.StatePending {
		if j != nil {
			return Task{}, ErrUnavailable
		}
		return dto, nil
	}
	if j == nil || m.JobID == nil || j.ID != *m.JobID || acquisition.ValidateLinkedAcquisitionJob(m.ID, *j) != nil || j.ClaimAttempts < 0 || j.FailureCount < 0 || j.MaxAttempts < 1 || j.FailureCount > j.MaxAttempts {
		return Task{}, ErrUnavailable
	}
	valid := false
	switch m.State {
	case acquisition.StateActive:
		valid = j.State == jobs.StateQueued || j.State == jobs.StateRunning || j.State == jobs.StateRetryWait
	case acquisition.StateAwaitingVisibility, acquisition.StateAwaitingCanonical:
		valid = j.State == jobs.StateRunning || j.State == jobs.StateRetryWait
	case acquisition.StateRecoveryRequired:
		valid = j.State == jobs.StateRecoveryRequired
	case acquisition.StateReady:
		valid = j.State == jobs.StateSucceeded
	case acquisition.StateFailed:
		valid = j.State == jobs.StateFailed
	case acquisition.StateCanceled:
		valid = j.State == jobs.StateCanceled
	}
	if !valid {
		return Task{}, ErrUnavailable
	}
	dto.JobState = &j.State
	dto.ClaimAttempts = j.ClaimAttempts
	dto.FailureCount = j.FailureCount
	return dto, nil
}
func (s *ReadService) binding(ctx context.Context, p auth.Principal, id storage.BindingID) (storage.Binding, error) {
	if err := principalValid(p); err != nil {
		return storage.Binding{}, err
	}
	if !ValidID(string(id)) {
		return storage.Binding{}, ErrInvalid
	}
	if !s.bindings[id] {
		return storage.Binding{}, ErrNotFound
	}
	binding, err := s.topology.GetBinding(ctx, id)
	if errors.Is(err, storage.ErrNotFound) {
		return storage.Binding{}, ErrNotFound
	}
	if err != nil {
		return storage.Binding{}, safeReadError(err)
	}
	if binding.ID != id || binding.IndexCoreRootID == "" || !binding.Status.Valid() {
		return storage.Binding{}, ErrUnavailable
	}
	return binding, nil
}
func (s *ReadService) Browse(ctx context.Context, p auth.Principal, request BrowseRequest) (ResourcePage, error) {
	if s == nil {
		return ResourcePage{}, ErrUnavailable
	}
	if request.WholeRoot && request.ParentResourceID != nil || request.Limit < 1 || request.Limit > 50 || !ValidOpaque(request.Cursor) || (request.ParentResourceID != nil && (*request.ParentResourceID == "" || !ValidOpaque(*request.ParentResourceID))) {
		return ResourcePage{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()
	binding, err := s.binding(ctx, p, request.BindingID)
	if err != nil {
		return ResourcePage{}, err
	}
	page, err := s.index.Browse(ctx, indexcore.BrowseRequest{WholeRoot: request.WholeRoot, RootID: binding.IndexCoreRootID, ParentResourceID: request.ParentResourceID, Cursor: request.Cursor, Limit: request.Limit})
	if err != nil {
		return ResourcePage{}, safeReadError(err)
	}
	if len(page.Items) > request.Limit || !ValidOpaque(page.NextCursor) || page.NextCursor != "" && page.NextCursor == request.Cursor {
		return ResourcePage{}, ErrUnavailable
	}
	result := ResourcePage{Items: make([]Resource, 0, len(page.Items)), NextCursor: page.NextCursor}
	seen := map[string]bool{}
	for _, physical := range page.Items {
		if seen[physical.ResourceID] {
			return ResourcePage{}, ErrUnavailable
		}
		seen[physical.ResourceID] = true
		item, err := resourceView(binding, physical, nil)
		if err != nil {
			return ResourcePage{}, err
		}
		copy, err := s.copies.GetCopyByPhysicalIdentity(ctx, physical.RootID, physical.ResourceID)
		if err == nil {
			item, err = resourceView(binding, physical, &copy)
		} else if errors.Is(err, catalog.ErrNotFound) {
			err = nil
		}
		if err != nil {
			return ResourcePage{}, safeReadError(err)
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}
func (s *ReadService) Resolve(ctx context.Context, p auth.Principal, id storage.BindingID, path string) (Resource, error) {
	if s == nil {
		return Resource{}, ErrUnavailable
	}
	normalized, err := acquisition.NormalizeTargetPath(path)
	if err != nil {
		return Resource{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()
	binding, err := s.binding(ctx, p, id)
	if err != nil {
		return Resource{}, err
	}
	detail, err := s.physical.Resolve(ctx, resourceview.PhysicalRequest{RootID: binding.IndexCoreRootID, Path: normalized})
	if err != nil {
		return Resource{}, safeReadError(err)
	}
	if detail.Ambiguous || len(detail.Matches) > 1 {
		return Resource{}, ErrAmbiguous
	}
	if len(detail.Matches) == 0 {
		return Resource{}, ErrNotFound
	}
	match := detail.Matches[0]
	if match.Physical.CanonicalPath == nil || *match.Physical.CanonicalPath != normalized {
		return Resource{}, ErrUnavailable
	}
	item, err := resourceView(binding, match.Physical, match.Copy)
	if err != nil {
		return Resource{}, err
	}
	switch match.Classification {
	case resourceview.ClassificationPhysicalOnly:
		if match.Copy != nil {
			return Resource{}, ErrUnavailable
		}
	case resourceview.ClassificationUnresolvedCopy:
		if match.Copy == nil || match.Copy.VariantID != nil {
			return Resource{}, ErrUnavailable
		}
	case resourceview.ClassificationClassified:
		if match.Copy == nil || match.Copy.VariantID == nil || match.Variant == nil || match.Release == nil || match.Asset == nil || *match.Copy.VariantID != match.Variant.ID || match.Variant.ReleaseID != match.Release.ID || match.Release.AssetID != match.Asset.ID {
			return Resource{}, ErrUnavailable
		}
		item.VariantID = &match.Variant.ID
		item.ReleaseID = &match.Release.ID
		item.AssetID = &match.Asset.ID
	default:
		return Resource{}, ErrUnavailable
	}
	return item, nil
}
func resourceView(binding storage.Binding, physical indexcore.ResourceContext, copy *catalog.Copy) (Resource, error) {
	if physical.RootID != binding.IndexCoreRootID || physical.ResourceID == "" || !ValidOpaque(physical.ResourceID) || physical.Presence != indexcore.ResourcePresent || physical.SizeBytes != nil && *physical.SizeBytes < 0 {
		return Resource{}, ErrUnavailable
	}
	if physical.CanonicalPath != nil {
		normalized, err := acquisition.NormalizeTargetPath(*physical.CanonicalPath)
		if err != nil || normalized != *physical.CanonicalPath {
			return Resource{}, ErrUnavailable
		}
	}
	if physical.Name != nil && (!utf8.ValidString(*physical.Name) || strings.ContainsRune(*physical.Name, 0) || len(*physical.Name) > 2048) {
		return Resource{}, ErrUnavailable
	}
	dto := Resource{BindingID: binding.ID, ResourceID: physical.ResourceID, Path: physical.CanonicalPath, Name: physical.Name, Directory: physical.Directory, SizeBytes: physical.SizeBytes, Presence: physical.Presence}
	if copy != nil {
		if !ValidID(string(copy.ID)) || copy.IndexCoreRootID != physical.RootID || copy.IndexCoreResourceID != physical.ResourceID || string(copy.StorageBindingID) != string(binding.ID) || (copy.Availability != catalog.CopyAvailabilityPresent && copy.Availability != catalog.CopyAvailabilityRemoved) {
			return Resource{}, ErrUnavailable
		}
		dto.CopyID = &copy.ID
		dto.CopyAvailability = &copy.Availability
	}
	return dto, nil
}
