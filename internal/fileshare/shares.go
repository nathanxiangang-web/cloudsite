// Package fileshare owns expiring product shares, not provider shares.
package fileshare

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/fileaccess"
	"github.com/nathanxiangang-web/panta/internal/product"
	"golang.org/x/crypto/bcrypt"
	"io"
	"time"
)

var ErrInvalid = errors.New("invalid share request")
var ErrDenied = errors.New("share unavailable")
var ErrUnavailable = errors.New("sharing unavailable")
var ErrLimit = errors.New("share limit reached")

type Record struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Size      int64              `json:"size_bytes"`
	CreatedAt time.Time          `json:"created_at"`
	ExpiresAt time.Time          `json:"expires_at"`
	Active    bool               `json:"active"`
	Protected bool               `json:"protected"`
	Owner     auth.UserID        `json:"-"`
	Target    fileaccess.Request `json:"-"`
	TokenHash []byte             `json:"-"`
	CodeHash  []byte             `json:"-"`
}
type Grant struct {
	Record    Record
	Principal auth.Principal
	Token     string
}
type Files interface {
	Describe(context.Context, auth.Principal, fileaccess.Request) (fileaccess.File, error)
	Open(context.Context, auth.Principal, fileaccess.Request) (fileaccess.File, error)
}
type Store interface {
	Create(context.Context, Record, time.Duration) (Record, error)
	List(context.Context, auth.UserID) ([]Record, error)
	Revoke(context.Context, auth.UserID, string) error
	Secret(context.Context, []byte) (Record, error)
	IssueGrant(context.Context, string, []byte) error
	Granted(context.Context, string, []byte) (Record, auth.Principal, error)
}
type Service struct {
	store Store
	files Files
	dummy []byte
}

func New(store Store, files Files) (*Service, error) {
	if store == nil || files == nil {
		return nil, ErrUnavailable
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("dummy-code-not-a-share"), bcrypt.DefaultCost)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Service{store, files, dummy}, nil
}
func SecretHash(s string) []byte { sum := sha256.Sum256([]byte(s)); return sum[:] }
func validSecret(s string) bool {
	b, e := base64.RawURLEncoding.DecodeString(s)
	return e == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == s
}
func secret() (string, error) {
	b := make([]byte, 32)
	if _, e := io.ReadFull(rand.Reader, b); e != nil {
		return "", ErrUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func identifier() (string, error) {
	var b [16]byte
	if _, e := io.ReadFull(rand.Reader, b[:]); e != nil {
		return "", ErrUnavailable
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func ValidCode(code string) bool {
	if len(code) < 6 || len(code) > 32 {
		return false
	}
	for _, c := range code {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func (s *Service) Create(ctx context.Context, p auth.Principal, target fileaccess.Request, duration, code string) (Record, string, error) {
	if !p.Valid() {
		return Record{}, "", auth.ErrUnauthorized
	}
	ttl := map[string]time.Duration{"5m": 5 * time.Minute, "1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour}[duration]
	if ttl == 0 || code != "" && !ValidCode(code) {
		return Record{}, "", ErrInvalid
	}
	target.Range = ""
	target.Head = false
	file, err := s.files.Describe(ctx, p, target)
	if err != nil {
		return Record{}, "", err
	}
	token, err := secret()
	if err != nil {
		return Record{}, "", err
	}
	id, err := identifier()
	if err != nil {
		return Record{}, "", err
	}
	record := Record{ID: id, Owner: p.UserID, Target: target, Name: file.Name, Size: file.Size, TokenHash: SecretHash(token), Protected: code != ""}
	if code != "" {
		record.CodeHash, err = bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return Record{}, "", ErrUnavailable
		}
	}
	record, err = s.store.Create(ctx, record, ttl)
	if err != nil {
		return Record{}, "", err
	}
	return record, token, nil
}
func (s *Service) List(ctx context.Context, p auth.Principal) ([]Record, error) {
	if !p.Valid() {
		return nil, auth.ErrUnauthorized
	}
	return s.store.List(ctx, p.UserID)
}
func (s *Service) Revoke(ctx context.Context, p auth.Principal, id string) error {
	if !p.Valid() {
		return auth.ErrUnauthorized
	}
	if !product.ValidID(id) {
		return ErrInvalid
	}
	return s.store.Revoke(ctx, p.UserID, id)
}
func (s *Service) Redeem(ctx context.Context, token, code string) (Grant, error) {
	if !validSecret(token) || len(code) > 32 {
		return Grant{}, ErrDenied
	}
	record, err := s.store.Secret(ctx, SecretHash(token))
	hash := s.dummy
	if err == nil && record.Protected {
		hash = record.CodeHash
	}
	match := bcrypt.CompareHashAndPassword(hash, []byte(code)) == nil
	if err != nil {
		if errors.Is(err, ErrDenied) {
			return Grant{}, ErrDenied
		}
		return Grant{}, ErrUnavailable
	}
	if record.Protected && !match {
		return Grant{}, ErrDenied
	}
	grant, err := secret()
	if err != nil {
		return Grant{}, err
	}
	if err = s.store.IssueGrant(ctx, record.ID, SecretHash(grant)); err != nil {
		return Grant{}, err
	}
	return Grant{Record: record, Token: grant}, nil
}
func (s *Service) Open(ctx context.Context, id, token, byteRange string, head bool) (fileaccess.File, error) {
	if !product.ValidID(id) || !validSecret(token) {
		return fileaccess.File{}, ErrDenied
	}
	record, principal, err := s.store.Granted(ctx, id, SecretHash(token))
	if err != nil {
		return fileaccess.File{}, err
	}
	target := record.Target
	target.Range = byteRange
	target.Head = head
	file, err := s.files.Open(ctx, principal, target)
	if err != nil {
		return file, err
	}
	if file.Size != record.Size || file.Name != record.Name {
		file.Content.Body.Close()
		return fileaccess.File{}, ErrDenied
	}
	return file, nil
}
