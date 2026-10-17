// Package auth owns product identity independently of provider credentials.
// Persistence and HTTP are ports/adapters outside this domain.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalid      = errors.New("invalid product authentication input")
	ErrUnauthorized = errors.New("product authentication required")
	ErrUnavailable  = errors.New("product authentication unavailable")
	ErrNotFound     = errors.New("product account not found")
	ErrConflict     = errors.New("product account already exists")
)

type UserID string
type Role string

const (
	RoleUser  Role = "USER"
	RoleAdmin Role = "ADMIN"
	// Fixed product-session lifetime; independent of provider cookie validity.
	SessionTTL = 30 * 24 * time.Hour
)

var loginPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)
var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func (role Role) Valid() bool { return role == RoleUser || role == RoleAdmin }
func NormalizeLogin(value string) (string, error) {
	value = strings.ToLower(value)
	if !loginPattern.MatchString(value) {
		return "", ErrInvalid
	}
	return value, nil
}
func ValidPassword(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) >= 6 && len(value) <= 72 && !strings.ContainsRune(value, 0)
}

type User struct {
	ID           UserID
	Login        string
	PasswordHash []byte
	Role         Role
	Enabled      bool
}
type Principal struct {
	UserID UserID `json:"user_id"`
	Login  string `json:"login"`
	Role   Role   `json:"role"`
}

func (principal Principal) Valid() bool {
	login, err := NormalizeLogin(principal.Login)
	return err == nil && login == principal.Login && idPattern.MatchString(string(principal.UserID)) && principal.Role.Valid()
}
func (user User) Valid() bool {
	_, err := bcrypt.Cost(user.PasswordHash)
	return Principal{user.ID, user.Login, user.Role}.Valid() && err == nil
}

type LoginResult struct {
	Principal Principal
	Token     string // secret; only cookie transport may return it
	ExpiresAt time.Time
}
type Store interface {
	CreateUser(context.Context, User) error
	UserByLogin(context.Context, string) (User, error)
	CreateSession(context.Context, UserID, []byte, []byte, time.Duration) (Principal, time.Time, error)
	SessionPrincipal(context.Context, []byte) (Principal, error)
	RevokeSession(context.Context, []byte) error
}
type Access interface {
	Login(context.Context, string, string) (LoginResult, error)
	Authenticate(context.Context, string) (Principal, error)
	Logout(context.Context, string) error
}

// Registrar cannot choose roles: self-service accounts are always ordinary users.
type Registrar interface {
	Register(context.Context, string, string) (Principal, error)
}
type Service struct {
	store     Store
	dummyHash []byte
}

func NewService(store Store) (*Service, error) {
	if store == nil || (reflect.ValueOf(store).Kind() == reflect.Ptr && reflect.ValueOf(store).IsNil()) {
		return nil, ErrUnavailable
	}
	// A valid dummy hash makes syntactically valid unknown-user logins perform
	// the same bcrypt comparison; this value is never a provisioned password.
	hash, err := bcrypt.GenerateFromPassword([]byte("product-auth-no-account-placeholder"), bcrypt.DefaultCost)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Service{store: store, dummyHash: hash}, nil
}
func NewUserID() (UserID, error) {
	var value [16]byte
	if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
		return "", ErrUnavailable
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	h := hex.EncodeToString(value[:])
	return UserID(h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]), nil
}

func (service *Service) Register(ctx context.Context, login, password string) (Principal, error) {
	return service.Provision(ctx, login, password, RoleUser)
}

// Provision is an operator application action. No public route exposes it.
// Duplicate login fails rather than silently resetting an existing password.
func (service *Service) Provision(ctx context.Context, login, password string, role Role) (Principal, error) {
	if service == nil || service.store == nil {
		return Principal{}, ErrUnavailable
	}
	login, err := NormalizeLogin(login)
	if err != nil || !ValidPassword(password) || !role.Valid() {
		return Principal{}, ErrInvalid
	}
	if err = ctx.Err(); err != nil {
		return Principal{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Principal{}, ErrUnavailable
	}
	id, err := NewUserID()
	if err != nil {
		return Principal{}, err
	}
	if err = ctx.Err(); err != nil {
		return Principal{}, err
	}
	user := User{ID: id, Login: login, PasswordHash: hash, Role: role, Enabled: true}
	if err = service.store.CreateUser(ctx, user); err != nil {
		return Principal{}, safeStoreError(err)
	}
	return Principal{id, login, role}, nil
}
func (service *Service) Login(ctx context.Context, login, password string) (LoginResult, error) {
	if service == nil || service.store == nil {
		return LoginResult{}, ErrUnavailable
	}
	login, err := NormalizeLogin(login)
	if err != nil || !ValidPassword(password) {
		return LoginResult{}, ErrUnauthorized
	}
	if err = ctx.Err(); err != nil {
		return LoginResult{}, err
	}
	user, err := service.store.UserByLogin(ctx, login)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return LoginResult{}, safeStoreError(err)
	}
	hash := service.dummyHash
	known := err == nil && user.Valid() && user.Login == login
	if known {
		hash = user.PasswordHash
	}
	matched := bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
	if !known || !user.Enabled || !matched {
		return LoginResult{}, ErrUnauthorized
	}
	if err = ctx.Err(); err != nil {
		return LoginResult{}, err
	}
	var entropy [32]byte
	if _, err = io.ReadFull(rand.Reader, entropy[:]); err != nil {
		return LoginResult{}, ErrUnavailable
	}
	token := base64.RawURLEncoding.EncodeToString(entropy[:])
	clear(entropy[:])
	digest := TokenDigest(token)
	principal, expires, err := service.store.CreateSession(ctx, user.ID, digest[:], user.PasswordHash, SessionTTL)
	if err != nil {
		return LoginResult{}, safeStoreError(err)
	}
	if !principal.Valid() || principal.UserID != user.ID || expires.IsZero() {
		return LoginResult{}, ErrUnavailable
	}
	return LoginResult{Principal: principal, Token: token, ExpiresAt: expires}, nil
}
func ValidToken(token string) bool {
	if len(token) != 43 {
		return false
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(token)
	valid := err == nil && len(data) == 32
	clear(data)
	return valid
}
func TokenDigest(token string) [32]byte { return sha256.Sum256([]byte(token)) }
func (service *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if service == nil || service.store == nil {
		return Principal{}, ErrUnavailable
	}
	if !ValidToken(token) {
		return Principal{}, ErrUnauthorized
	}
	if err := ctx.Err(); err != nil {
		return Principal{}, err
	}
	digest := TokenDigest(token)
	principal, err := service.store.SessionPrincipal(ctx, digest[:])
	if err != nil {
		return Principal{}, safeStoreError(err)
	}
	if !principal.Valid() {
		return Principal{}, ErrUnavailable
	}
	return principal, nil
}
func (service *Service) Logout(ctx context.Context, token string) error {
	if service == nil || service.store == nil {
		return ErrUnavailable
	}
	if !ValidToken(token) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	digest := TokenDigest(token)
	return safeStoreError(service.store.RevokeSession(ctx, digest[:]))
}
func safeStoreError(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{ErrConflict, ErrUnauthorized, ErrUnavailable, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			return known
		}
	}
	return ErrUnavailable // never publish arbitrary persistence error strings
}
