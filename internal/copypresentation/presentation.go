// Package copypresentation owns product display metadata, never physical identity,
// file names, Canonical paths, availability or access-control policy.
package copypresentation

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxVersion int64 = 9007199254740991

type Change struct {
	BindingID   string `json:"binding_id"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
	Hidden      bool   `json:"hidden"`
}

func ValidName(s string) bool {
	if !utf8.ValidString(s) || len(s) > 640 || utf8.RuneCountInString(s) > 160 || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '<' || r == '>' || r == '\ufffd' {
			return false
		}
	}
	return true
}
func ValidCategory(s string) bool {
	switch s {
	case "AUTO", "SOFTWARE", "VIDEO", "DOCUMENT", "IMAGE", "AUDIO", "ARCHIVE", "OTHER":
		return true
	}
	return false
}
func (c Change) Valid() bool { return ValidName(c.DisplayName) && ValidCategory(c.Category) }

type Overlay struct {
	DisplayName string     `json:"display_name"`
	Category    string     `json:"category"`
	Hidden      bool       `json:"hidden"`
	Version     int64      `json:"version"`
	UpdatedAt   *time.Time `json:"updated_at"`
	UpdatedBy   *string    `json:"updated_by"`
}

func (o Overlay) Valid() bool {
	if !ValidName(o.DisplayName) || !ValidCategory(o.Category) || o.Version < 0 || o.Version > MaxVersion {
		return false
	}
	if o.Version == 0 {
		return o.DisplayName == "" && o.Category == "AUTO" && !o.Hidden && o.UpdatedAt == nil && o.UpdatedBy == nil
	}
	return o.UpdatedAt != nil && !o.UpdatedAt.IsZero() && o.UpdatedBy != nil
}

type Evidence struct {
	CopyID           string    `json:"copy_id"`
	BindingID        string    `json:"binding_id"`
	RootID           string    `json:"indexcore_root_id"`
	ResourceID       string    `json:"indexcore_resource_id"`
	Availability     string    `json:"availability"`
	ProjectionCursor int64     `json:"projection_cursor"`
	CopyUpdatedAt    time.Time `json:"copy_updated_at"`
}
type Detail struct {
	Evidence          Evidence  `json:"evidence"`
	Presentation      Overlay   `json:"presentation"`
	Source            string    `json:"source"`
	VisibilityEffect  string    `json:"visibility_effect"`
	EmptyNameFallback string    `json:"empty_name_fallback"`
	CanEdit           bool      `json:"can_edit"`
	ReadAt            time.Time `json:"read_at"`
}
