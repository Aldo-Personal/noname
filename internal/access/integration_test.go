package access

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"infra.local/platform/internal/platform/database"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := database.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + randomToken(8)
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	pool, err := database.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migration not idempotent: %v", err)
	}
	if err := database.Check(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return &Store{Pool: pool}
}
func principal(t *testing.T, s *Store, subject string) (Principal, string) {
	t.Helper()
	p, token, err := s.Login(context.Background(), "https://issuer.test", subject, subject+"@example.test")
	if err != nil {
		t.Fatal(err)
	}
	return p, token
}

func TestTenantAndKeyLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	alice, session := principal(t, s, "alice")
	bob, _ := principal(t, s, "bob")
	project, err := s.CreateProject(ctx, alice.OrganizationID, "Ethereum app")
	if err != nil {
		t.Fatal(err)
	}
	if project.ChainID != 1 {
		t.Fatal("wrong chain")
	}
	issued, err := s.IssueKey(ctx, alice.OrganizationID, project.ID, "Server key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Keys(ctx, bob.OrganizationID, project.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant read: %v", err)
	}
	if _, err := s.IssueKey(ctx, bob.OrganizationID, project.ID, "Bad"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant issue: %v", err)
	}
	if err := s.RevokeKey(ctx, bob.OrganizationID, project.ID, issued.Key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant revoke: %v", err)
	}
	if _, err := s.RotateKey(ctx, bob.OrganizationID, project.ID, issued.Key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant rotation: %v", err)
	}
	identity, err := s.AuthenticateKey(ctx, issued.Secret)
	if err != nil || identity.ProjectID != project.ID {
		t.Fatalf("key auth: %v", err)
	}
	// Recreate the service to ensure no in-memory session or key state is required.
	restarted := &Store{Pool: s.Pool}
	if _, err := restarted.Session(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.AuthenticateKey(ctx, issued.Secret); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := s.Pool.QueryRow(ctx, `SELECT token_hash FROM api_keys WHERE id=$1`, issued.Key.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 32 || string(stored) == issued.Secret {
		t.Fatal("key not hashed")
	}
	rotated, err := s.RotateKey(ctx, alice.OrganizationID, project.ID, issued.Key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateKey(ctx, issued.Secret); !errors.Is(err, ErrNotFound) {
		t.Fatal("old key still accepted")
	}
	if err := s.RevokeKey(ctx, alice.OrganizationID, project.ID, rotated.Key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateKey(ctx, rotated.Secret); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked key accepted")
	}
	if _, err := s.AuthenticateKey(ctx, "infra_sk_"+strings.Repeat("0", 64)); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown key accepted")
	}
	expired, err := s.IssueKey(ctx, alice.OrganizationID, project.ID, "Expired")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE api_keys SET expires_at=now()-interval '1 second' WHERE id=$1`, expired.Key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateKey(ctx, expired.Secret); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired key accepted")
	}
	if err := s.Logout(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(ctx, session); !errors.Is(err, ErrNotFound) {
		t.Fatal("logged out session accepted")
	}
}

func TestConcurrentRotationHasOneWinner(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p, _ := principal(t, s, "alice")
	project, err := s.CreateProject(ctx, p.OrganizationID, "App")
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.IssueKey(ctx, p.OrganizationID, project.ID, "Key")
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.RotateKey(ctx, p.OrganizationID, project.ID, key.Key.ID)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("rotation winners %d", winners)
	}
}

func TestHTTPAuthorizationAndValidation(t *testing.T) {
	s := testStore(t)
	p, session := principal(t, s, "alice")
	_, otherSession := principal(t, s, "bob")
	app := &App{store: s, config: Config{Origin: "http://localhost:5173"}}
	handler := app.Routes()
	request := func(method, path, body, token, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "infra_session", Value: token})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/v1/projects", "", "", ""); w.Code != 401 {
		t.Fatalf("anonymous status %d", w.Code)
	}
	if w := request("POST", "/v1/projects", `{"name":"App"}`, session, "https://attacker.test"); w.Code != 403 {
		t.Fatalf("CSRF accepted: %d", w.Code)
	}
	if w := request("POST", "/v1/projects", `{"name":"App","organizationId":"forged"}`, session, app.config.Origin); w.Code != 400 {
		t.Fatal("unknown fields accepted")
	}
	if w := request("POST", "/v1/projects", `{"name":"App"} {}`, session, app.config.Origin); w.Code != 400 {
		t.Fatal("extra JSON accepted")
	}
	w := request("POST", "/v1/projects", `{"name":"App"}`, session, app.config.Origin)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var project Project
	if err := json.Unmarshal(w.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	w = request("GET", "/v1/projects/"+project.ID+"/keys", "", otherSession, "")
	if w.Code != 404 {
		t.Fatal("cross-tenant keys exposed")
	}
	issued, err := s.IssueKey(context.Background(), p.OrganizationID, project.ID, "Server")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/v1/key-check", nil)
	req.Header.Set("Authorization", "Bearer "+issued.Secret)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = request("GET", "/v1/projects/"+project.ID+"/keys", "", session, "")
	if strings.Contains(w.Body.String(), issued.Secret) || strings.Contains(w.Body.String(), "token_hash") {
		t.Fatal("key material exposed")
	}
	if _, err := s.Pool.Exec(context.Background(), `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, tokenHash(session)); err != nil {
		t.Fatal(err)
	}
	if w := request("GET", "/v1/me", "", session, ""); w.Code != 401 {
		t.Fatal("expired session accepted")
	}
}

func TestMigrationTamperingRejected(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `UPDATE schema_migrations SET checksum='bad'::bytea`); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, s.Pool); err == nil {
		t.Fatal("modified applied migration accepted")
	}
}

type identityFixture struct {
	mu                     sync.Mutex
	nonce, challenge, mode string
	key                    *rsa.PrivateKey
	server                 *httptest.Server
}

func fixture(t *testing.T) *identityFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &identityFixture{key: key}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": f.server.URL, "authorization_endpoint": f.server.URL + "/authorize", "token_endpoint": f.server.URL + "/token", "jwks_uri": f.server.URL + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(f.key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(f.key.E)).Bytes())}}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", 400)
				return
			}
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "test-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				http.Error(w, "bad PKCE", 400)
				return
			}
			claims := map[string]any{"iss": f.server.URL, "sub": "oidc-alice", "aud": "dashboard", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": f.nonce, "email": "alice@example.test"}
			switch f.mode {
			case "nonce":
				claims["nonce"] = "wrong"
			case "audience":
				claims["aud"] = "other"
			case "issuer":
				claims["iss"] = "https://wrong.test"
			case "expiry":
				claims["exp"] = time.Now().Add(-time.Hour).Unix()
			}
			header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test"})
			payload, _ := json.Marshal(claims)
			input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
			digest := sha256.Sum256([]byte(input))
			sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
			if err != nil {
				http.Error(w, "signing failed", 500)
				return
			}
			if f.mode == "signature" {
				sig[0] ^= 1
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "not-stored", "token_type": "Bearer", "expires_in": 3600, "id_token": input + "." + base64.RawURLEncoding.EncodeToString(sig)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func TestOIDCLoginAndInvalidIdentity(t *testing.T) {
	s := testStore(t)
	f := fixture(t)
	a, err := New(context.Background(), s, Config{Issuer: f.server.URL, ClientID: "dashboard", Origin: "http://localhost:5173"})
	if err != nil {
		t.Fatal(err)
	}
	h := a.Routes()
	for _, mode := range []string{"valid", "nonce", "audience", "issuer", "expiry", "signature", "state", "expired-state", "pkce"} {
		t.Run(mode, func(t *testing.T) {
			login := httptest.NewRecorder()
			h.ServeHTTP(login, httptest.NewRequest("GET", "/auth/login", nil))
			if login.Code != 302 {
				t.Fatalf("login: %d", login.Code)
			}
			location, err := url.Parse(login.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			query := location.Query()
			if query.Get("code_challenge_method") != "S256" {
				t.Fatal("PKCE missing")
			}
			f.mu.Lock()
			f.nonce = query.Get("nonce")
			f.challenge = query.Get("code_challenge")
			f.mode = mode
			if mode == "pkce" {
				f.challenge = "wrong"
			}
			f.mu.Unlock()
			state := query.Get("state")
			if mode == "state" {
				state = strings.Repeat("0", 64)
			}
			if mode == "expired-state" {
				if _, err := s.Pool.Exec(context.Background(), `UPDATE login_flows SET expires_at=now()-interval '1 second' WHERE state_hash=$1`, tokenHash(state)); err != nil {
					t.Fatal(err)
				}
			}
			req := httptest.NewRequest("GET", "/auth/callback?code=test-code&state="+state, nil)
			for _, cookie := range login.Result().Cookies() {
				req.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			h.ServeHTTP(response, req)
			if mode != "valid" {
				if response.Code != 400 && response.Code != 401 {
					t.Fatalf("invalid %s accepted: %d", mode, response.Code)
				}
				return
			}
			if response.Code != 302 {
				t.Fatalf("callback: %d %s", response.Code, response.Body.String())
			}
			var session *http.Cookie
			for _, c := range response.Result().Cookies() {
				if c.Name == "infra_session" {
					session = c
				}
			}
			if session == nil || !session.HttpOnly || session.SameSite != http.SameSiteLaxMode {
				t.Fatal("session cookie missing protections")
			}
			if _, err := s.Session(context.Background(), session.Value); err != nil {
				t.Fatal(err)
			}
			replay := httptest.NewRecorder()
			h.ServeHTTP(replay, req)
			if replay.Code != 400 {
				t.Fatal("login callback replay accepted")
			}
		})
	}
}

func TestInvalidNames(t *testing.T) {
	for _, name := range []string{"", " ", " App", strings.Repeat("x", 81)} {
		if validName(name) {
			t.Fatal(fmt.Sprintf("accepted %q", name))
		}
	}
}
