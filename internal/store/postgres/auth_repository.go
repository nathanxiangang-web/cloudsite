package postgres

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nathanxiangang-web/panta/internal/auth"
)

type authDB interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
type AuthRepository struct{ db authDB }

var _ auth.Store = (*AuthRepository)(nil)

func NewAuthRepository(db authDB) (*AuthRepository, error) {
	if db == nil || (reflect.ValueOf(db).Kind() == reflect.Ptr && reflect.ValueOf(db).IsNil()) {
		return nil, auth.ErrUnavailable
	}
	return &AuthRepository{db: db}, nil
}
func authWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		return auth.ErrConflict
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return auth.ErrUnavailable
}
func (repo *AuthRepository) CreateUser(ctx context.Context, user auth.User) error {
	if !user.Valid() {
		return auth.ErrInvalid
	}
	_, err := repo.db.Exec(ctx, `INSERT INTO product_users(user_id,login,password_hash,role,enabled,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,clock_timestamp(),clock_timestamp())`, string(user.ID), user.Login, user.PasswordHash, string(user.Role), user.Enabled)
	return authWriteError(err)
}
func (repo *AuthRepository) UserByLogin(ctx context.Context, login string) (auth.User, error) {
	var user auth.User
	var id string
	err := repo.db.QueryRow(ctx, `SELECT user_id::text,login,password_hash,role,enabled FROM product_users WHERE login=$1`, login).Scan(&id, &user.Login, &user.PasswordHash, &user.Role, &user.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.User{}, authWriteError(err)
	}
	user.ID = auth.UserID(id)
	return user, nil
}
func (repo *AuthRepository) CreateSession(ctx context.Context, id auth.UserID, digest, expectedHash []byte, ttl time.Duration) (auth.Principal, time.Time, error) {
	if len(digest) != 32 || ttl != auth.SessionTTL {
		return auth.Principal{}, time.Time{}, auth.ErrInvalid
	}
	tx, err := repo.db.Begin(ctx)
	if err != nil {
		return auth.Principal{}, time.Time{}, authWriteError(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var principal auth.Principal
	var storedID string
	// Coordinate account disabling/password changes with session creation.
	err = tx.QueryRow(ctx, `SELECT user_id::text,login,role FROM product_users WHERE user_id=$1 AND enabled AND password_hash=$2 FOR SHARE`, string(id), expectedHash).Scan(&storedID, &principal.Login, &principal.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, time.Time{}, auth.ErrUnauthorized
	}
	if err != nil {
		return auth.Principal{}, time.Time{}, authWriteError(err)
	}
	principal.UserID = auth.UserID(storedID)
	if !principal.Valid() {
		return auth.Principal{}, time.Time{}, auth.ErrUnavailable
	}
	var expires time.Time
	err = tx.QueryRow(ctx, `WITH stamp AS (SELECT clock_timestamp() AS at)
INSERT INTO product_sessions(token_hash,user_id,created_at,expires_at)
SELECT $1,$2,at,at+($3*interval '1 second') FROM stamp RETURNING expires_at`, digest, string(id), ttl.Seconds()).Scan(&expires)
	if err != nil {
		return auth.Principal{}, time.Time{}, authWriteError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return auth.Principal{}, time.Time{}, authWriteError(err)
	}
	return principal, expires, nil
}
func (repo *AuthRepository) SessionPrincipal(ctx context.Context, digest []byte) (auth.Principal, error) {
	if len(digest) != 32 {
		return auth.Principal{}, auth.ErrUnauthorized
	}
	var principal auth.Principal
	var id string
	err := repo.db.QueryRow(ctx, `SELECT u.user_id::text,u.login,u.role FROM product_sessions s
JOIN product_users u ON u.user_id=s.user_id
WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.enabled`, digest).Scan(&id, &principal.Login, &principal.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Principal{}, auth.ErrUnauthorized
	}
	if err != nil {
		return auth.Principal{}, authWriteError(err)
	}
	principal.UserID = auth.UserID(id)
	if !principal.Valid() {
		return auth.Principal{}, auth.ErrUnavailable
	}
	return principal, nil
}
func (repo *AuthRepository) RevokeSession(ctx context.Context, digest []byte) error {
	if len(digest) != 32 {
		return nil
	}
	_, err := repo.db.Exec(ctx, `UPDATE product_sessions SET revoked_at=clock_timestamp() WHERE token_hash=$1 AND revoked_at IS NULL`, digest)
	return authWriteError(err)
}
