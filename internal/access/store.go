// Package access owns personal organizations, projects, sessions, and project keys.
package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")
var ErrInvalid = errors.New("invalid input")

type Store struct{ Pool *pgxpool.Pool }
type Principal struct {
	UserID         string `json:"userId"`
	OrganizationID string `json:"organizationId"`
	Email          string `json:"email"`
}
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ChainID   int64     `json:"chainId"`
	CreatedAt time.Time `json:"createdAt"`
}
type Key struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"projectId"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt time.Time  `json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt"`
}
type IssuedKey struct {
	Key    Key    `json:"key"`
	Secret string `json:"secret"`
}
type KeyIdentity struct {
	KeyID          string `json:"keyId"`
	ProjectID      string `json:"projectId"`
	OrganizationID string `json:"organizationId"`
	ChainID        int64  `json:"chainId"`
}

func randomToken(bytes int) string { return hex.EncodeToString(randomBytes(bytes)) }
func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
func tokenHash(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }
func validName(name string) bool {
	return name == strings.TrimSpace(name) && utf8.RuneCountInString(name) >= 1 && utf8.RuneCountInString(name) <= 80
}
func normalize(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Store) Login(ctx context.Context, issuer, subject, email string) (Principal, string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Principal{}, "", err
	}
	defer tx.Rollback(ctx)
	var p Principal
	err = tx.QueryRow(ctx, `INSERT INTO users(id,issuer,subject,email) VALUES($1,$2,$3,$4) ON CONFLICT(issuer,subject) DO UPDATE SET email=EXCLUDED.email RETURNING id,email`, randomToken(16), issuer, subject, email).Scan(&p.UserID, &p.Email)
	if err != nil {
		return p, "", err
	}
	err = tx.QueryRow(ctx, `INSERT INTO organizations(id,owner_id,name) VALUES($1,$2,'Personal workspace') ON CONFLICT(owner_id) DO UPDATE SET owner_id=EXCLUDED.owner_id RETURNING id`, randomToken(16), p.UserID).Scan(&p.OrganizationID)
	if err != nil {
		return p, "", err
	}
	token := randomToken(32)
	_, err = tx.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '24 hours')`, tokenHash(token), p.UserID)
	if err != nil {
		return p, "", err
	}
	return p, token, tx.Commit(ctx)
}
func (s *Store) Session(ctx context.Context, token string) (Principal, error) {
	var p Principal
	if len(token) != 64 {
		return p, ErrNotFound
	}
	err := s.Pool.QueryRow(ctx, `SELECT u.id,o.id,u.email FROM sessions s JOIN users u ON u.id=s.user_id JOIN organizations o ON o.owner_id=u.id WHERE s.token_hash=$1 AND s.expires_at>now()`, tokenHash(token)).Scan(&p.UserID, &p.OrganizationID, &p.Email)
	return p, normalize(err)
}
func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash(token))
	return err
}
func (s *Store) CreateProject(ctx context.Context, org, name string) (Project, error) {
	var p Project
	if !validName(name) {
		return p, ErrInvalid
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO projects(id,organization_id,name) VALUES($1,$2,$3) RETURNING id,name,chain_id,created_at`, randomToken(16), org, name).Scan(&p.ID, &p.Name, &p.ChainID, &p.CreatedAt)
	return p, err
}
func (s *Store) Projects(ctx context.Context, org string) ([]Project, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,name,chain_id,created_at FROM projects WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 100`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.ChainID, &p.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
func (s *Store) owns(ctx context.Context, org, project string) error {
	var id string
	return normalize(s.Pool.QueryRow(ctx, `SELECT id FROM projects WHERE id=$1 AND organization_id=$2`, project, org).Scan(&id))
}
func (s *Store) Keys(ctx context.Context, org, project string) ([]Key, error) {
	if err := s.owns(ctx, org, project); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id,project_id,name,prefix,created_at,expires_at,revoked_at FROM api_keys WHERE project_id=$1 ORDER BY created_at DESC LIMIT 100`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Key{}
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.Prefix, &k.CreatedAt, &k.ExpiresAt, &k.RevokedAt); err != nil {
			return nil, err
		}
		result = append(result, k)
	}
	return result, rows.Err()
}
func issue(ctx context.Context, tx pgx.Tx, project, name string) (IssuedKey, error) {
	result := IssuedKey{Secret: "infra_sk_" + randomToken(32)}
	k := &result.Key
	err := tx.QueryRow(ctx, `INSERT INTO api_keys(id,project_id,name,token_hash,prefix,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '90 days') RETURNING id,project_id,name,prefix,created_at,expires_at,revoked_at`, randomToken(16), project, name, tokenHash(result.Secret), result.Secret[:17]).Scan(&k.ID, &k.ProjectID, &k.Name, &k.Prefix, &k.CreatedAt, &k.ExpiresAt, &k.RevokedAt)
	return result, err
}
func (s *Store) IssueKey(ctx context.Context, org, project, name string) (IssuedKey, error) {
	if !validName(name) {
		return IssuedKey{}, ErrInvalid
	}
	if err := s.owns(ctx, org, project); err != nil {
		return IssuedKey{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return IssuedKey{}, err
	}
	defer tx.Rollback(ctx)
	result, err := issue(ctx, tx, project, name)
	if err != nil {
		return IssuedKey{}, err
	}
	return result, tx.Commit(ctx)
}
func (s *Store) RevokeKey(ctx context.Context, org, project, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE api_keys k SET revoked_at=coalesce(k.revoked_at,now()) FROM projects p WHERE k.project_id=p.id AND p.organization_id=$1 AND p.id=$2 AND k.id=$3`, org, project, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) RotateKey(ctx context.Context, org, project, id string) (IssuedKey, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return IssuedKey{}, err
	}
	defer tx.Rollback(ctx)
	var name string
	err = tx.QueryRow(ctx, `SELECT k.name FROM api_keys k JOIN projects p ON p.id=k.project_id WHERE p.organization_id=$1 AND p.id=$2 AND k.id=$3 AND k.revoked_at IS NULL AND k.expires_at>now() FOR UPDATE OF k`, org, project, id).Scan(&name)
	if err != nil {
		return IssuedKey{}, normalize(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE api_keys SET revoked_at=now() WHERE id=$1`, id); err != nil {
		return IssuedKey{}, err
	}
	result, err := issue(ctx, tx, project, name)
	if err != nil {
		return IssuedKey{}, err
	}
	return result, tx.Commit(ctx)
}
func (s *Store) AuthenticateKey(ctx context.Context, token string) (KeyIdentity, error) {
	var identity KeyIdentity
	if len(token) != 73 || !strings.HasPrefix(token, "infra_sk_") {
		return identity, ErrNotFound
	}
	err := s.Pool.QueryRow(ctx, `SELECT k.id,p.id,p.organization_id,p.chain_id FROM api_keys k JOIN projects p ON p.id=k.project_id WHERE k.token_hash=$1 AND k.revoked_at IS NULL AND k.expires_at>now()`, tokenHash(token)).Scan(&identity.KeyID, &identity.ProjectID, &identity.OrganizationID, &identity.ChainID)
	return identity, normalize(err)
}
