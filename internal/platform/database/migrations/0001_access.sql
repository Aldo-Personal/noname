CREATE TABLE users (
 id text PRIMARY KEY, issuer text NOT NULL, subject text NOT NULL,
 email text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(issuer, subject)
);
CREATE TABLE organizations (
 id text PRIMARY KEY, owner_id text NOT NULL UNIQUE REFERENCES users(id),
 name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
 token_hash bytea PRIMARY KEY, user_id text NOT NULL REFERENCES users(id),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_expiry ON sessions(expires_at);
CREATE TABLE login_flows (
 state_hash bytea PRIMARY KEY, nonce text NOT NULL, verifier text NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE INDEX login_flows_expiry ON login_flows(expires_at);
CREATE TABLE projects (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id),
 name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
 chain_id bigint NOT NULL DEFAULT 1 CHECK(chain_id = 1),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX projects_organization ON projects(organization_id, created_at);
CREATE TABLE api_keys (
 id text PRIMARY KEY, project_id text NOT NULL REFERENCES projects(id),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 80),
 token_hash bytea NOT NULL UNIQUE, prefix text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL,
 revoked_at timestamptz
);
CREATE INDEX api_keys_project ON api_keys(project_id, created_at);
