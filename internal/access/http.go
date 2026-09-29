package access

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type Config struct{ Issuer, ClientID, ClientSecret, Origin string }
type App struct {
	store    *Store
	config   Config
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	client   *http.Client
	secure   bool
}

func New(ctx context.Context, store *Store, config Config) (*App, error) {
	origin, err := url.Parse(config.Origin)
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" || (origin.Scheme != "http" && origin.Scheme != "https") {
		return nil, fmt.Errorf("PUBLIC_ORIGIN must be an HTTP(S) origin without a path")
	}
	if origin.Scheme == "http" && origin.Hostname() != "localhost" && origin.Hostname() != "127.0.0.1" {
		return nil, fmt.Errorf("PUBLIC_ORIGIN requires HTTPS outside loopback development")
	}
	issuer, err := url.Parse(config.Issuer)
	if err != nil || issuer.Host == "" || issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" || (issuer.Scheme != "https" && !(issuer.Scheme == "http" && (issuer.Hostname() == "localhost" || issuer.Hostname() == "127.0.0.1"))) {
		return nil, fmt.Errorf("OIDC_ISSUER requires HTTPS or loopback HTTP")
	}
	if config.ClientID == "" {
		return nil, fmt.Errorf("OIDC_CLIENT_ID is required")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), config.Issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery failed: %w", err)
	}
	return &App{store: store, config: config, client: client, secure: origin.Scheme == "https", verifier: provider.Verifier(&oidc.Config{ClientID: config.ClientID}), oauth: oauth2.Config{ClientID: config.ClientID, ClientSecret: config.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: config.Origin + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}}, nil
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("response write failed")
	}
}
func failure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		write(w, 404, map[string]string{"code": "not_found"})
	case errors.Is(err, ErrInvalid):
		write(w, 400, map[string]string{"code": "invalid_input"})
	default:
		slog.Error("access operation failed")
		write(w, 503, map[string]string{"code": "temporarily_unavailable"})
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return ErrInvalid
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return ErrInvalid
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return ErrInvalid
	}
	return nil
}
func (a *App) cookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func (a *App) authorized(fn func(http.ResponseWriter, *http.Request, Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != a.config.Origin {
			write(w, 403, map[string]string{"code": "invalid_origin"})
			return
		}
		cookie, err := r.Cookie("infra_session")
		if err != nil {
			write(w, 401, map[string]string{"code": "unauthenticated"})
			return
		}
		p, err := a.store.Session(r.Context(), cookie.Value)
		if errors.Is(err, ErrNotFound) {
			write(w, 401, map[string]string{"code": "unauthenticated"})
			return
		}
		if err != nil {
			failure(w, err)
			return
		}
		fn(w, r, p)
	}
}
func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	a.usageRoutes(mux)
	mux.HandleFunc("GET /auth/login", a.login)
	mux.HandleFunc("GET /auth/callback", a.callback)
	mux.HandleFunc("POST /auth/logout", a.authorized(func(w http.ResponseWriter, r *http.Request, p Principal) {
		c, _ := r.Cookie("infra_session")
		if err := a.store.Logout(r.Context(), c.Value); err != nil {
			failure(w, err)
			return
		}
		a.cookie(w, "infra_session", "", -1)
		write(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /v1/me", a.authorized(func(w http.ResponseWriter, r *http.Request, p Principal) { write(w, 200, p) }))
	mux.HandleFunc("GET /v1/projects", a.authorized(func(w http.ResponseWriter, r *http.Request, p Principal) {
		projects, err := a.store.Projects(r.Context(), p.OrganizationID)
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 200, map[string]any{"projects": projects})
	}))
	mux.HandleFunc("POST /v1/projects", a.authorized(func(w http.ResponseWriter, r *http.Request, p Principal) {
		var body struct {
			Name string `json:"name"`
		}
		if err := decode(w, r, &body); err != nil {
			failure(w, err)
			return
		}
		project, err := a.store.CreateProject(r.Context(), p.OrganizationID, body.Name)
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 201, project)
	}))
	mux.HandleFunc("GET /v1/projects/{project}/keys", a.authorized(func(w http.ResponseWriter, r *http.Request, p Principal) {
		keys, err := a.store.Keys(r.Context(), p.OrganizationID, r.PathValue("project"))
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 200, map[string]any{"keys": keys})
	}))
	mux.HandleFunc("POST /v1/projects/{project}/keys", a.authorized(func(w http.ResponseWriter, r *http.Request, p Principal) {
		var body struct {
			Name string `json:"name"`
		}
		if err := decode(w, r, &body); err != nil {
			failure(w, err)
			return
		}
		key, err := a.store.IssueKey(r.Context(), p.OrganizationID, r.PathValue("project"), body.Name)
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 201, key)
	}))
	mux.HandleFunc("DELETE /v1/projects/{project}/keys/{key}", a.authorized(func(w http.ResponseWriter, r *http.Request, p Principal) {
		if err := a.store.RevokeKey(r.Context(), p.OrganizationID, r.PathValue("project"), r.PathValue("key")); err != nil {
			failure(w, err)
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /v1/projects/{project}/keys/{key}/rotate", a.authorized(func(w http.ResponseWriter, r *http.Request, p Principal) {
		key, err := a.store.RotateKey(r.Context(), p.OrganizationID, r.PathValue("project"), r.PathValue("key"))
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 201, key)
	}))
	mux.HandleFunc("GET /v1/key-check", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			write(w, 401, map[string]string{"code": "invalid_api_key"})
			return
		}
		identity, err := a.store.AuthenticateKey(r.Context(), strings.TrimPrefix(auth, "Bearer "))
		if errors.Is(err, ErrNotFound) {
			write(w, 401, map[string]string{"code": "invalid_api_key"})
			return
		}
		if err != nil {
			failure(w, err)
			return
		}
		write(w, 200, identity)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	state, nonce, verifier := randomToken(32), randomToken(32), oauth2.GenerateVerifier()
	_, err := a.store.Pool.Exec(r.Context(), `INSERT INTO login_flows(state_hash,nonce,verifier,expires_at) VALUES($1,$2,$3,now()+interval '10 minutes')`, tokenHash(state), nonce, verifier)
	if err != nil {
		failure(w, err)
		return
	}
	a.cookie(w, "infra_login", state, 600)
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}
func (a *App) callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie("infra_login")
	if err != nil || len(state) != 64 || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		write(w, 400, map[string]string{"code": "invalid_login_state"})
		return
	}
	a.cookie(w, "infra_login", "", -1)
	var nonce, verifier string
	err = a.store.Pool.QueryRow(r.Context(), `DELETE FROM login_flows WHERE state_hash=$1 AND expires_at>now() RETURNING nonce,verifier`, tokenHash(state)).Scan(&nonce, &verifier)
	if err != nil {
		write(w, 400, map[string]string{"code": "expired_login"})
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		write(w, 400, map[string]string{"code": "login_denied"})
		return
	}
	token, err := a.oauth.Exchange(oidc.ClientContext(r.Context(), a.client), code, oauth2.VerifierOption(verifier))
	if err != nil {
		write(w, 401, map[string]string{"code": "login_failed"})
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		write(w, 401, map[string]string{"code": "invalid_identity"})
		return
	}
	identity, err := a.verifier.Verify(oidc.ClientContext(r.Context(), a.client), raw)
	if err != nil || identity.Subject == "" || subtle.ConstantTimeCompare([]byte(identity.Nonce), []byte(nonce)) != 1 {
		write(w, 401, map[string]string{"code": "invalid_identity"})
		return
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := identity.Claims(&claims); err != nil {
		write(w, 401, map[string]string{"code": "invalid_identity"})
		return
	}
	_, session, err := a.store.Login(r.Context(), identity.Issuer, identity.Subject, claims.Email)
	if err != nil {
		failure(w, err)
		return
	}
	// Reauthentication replaces this browser's previous session, if any.
	if old, err := r.Cookie("infra_session"); err == nil {
		if err := a.store.Logout(r.Context(), old.Value); err != nil {
			failure(w, err)
			return
		}
	}
	a.cookie(w, "infra_session", session, 86400)
	http.Redirect(w, r, a.config.Origin+"/", http.StatusFound)
}
