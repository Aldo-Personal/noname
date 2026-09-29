CREATE TABLE project_limits (
 project_id text PRIMARY KEY REFERENCES projects(id),
 daily_units bigint NOT NULL DEFAULT 10000 CHECK(daily_units BETWEEN 0 AND 100000),
 minute_requests bigint NOT NULL DEFAULT 60 CHECK(minute_requests BETWEEN 0 AND 600),
 minute_start timestamptz NOT NULL DEFAULT '1970-01-01 00:00:00+00',
 minute_used bigint NOT NULL DEFAULT 0 CHECK(minute_used >= 0)
);
CREATE TABLE usage_days (
 project_id text NOT NULL REFERENCES projects(id), day date NOT NULL,
 units bigint NOT NULL DEFAULT 0 CHECK(units >= 0),
 pending bigint NOT NULL DEFAULT 0 CHECK(pending >= 0),
 succeeded bigint NOT NULL DEFAULT 0 CHECK(succeeded >= 0),
 upstream_error bigint NOT NULL DEFAULT 0 CHECK(upstream_error >= 0),
 timed_out bigint NOT NULL DEFAULT 0 CHECK(timed_out >= 0),
 canceled bigint NOT NULL DEFAULT 0 CHECK(canceled >= 0),
 unknown bigint NOT NULL DEFAULT 0 CHECK(unknown >= 0),
 PRIMARY KEY(project_id,day),
 CHECK(units = pending + succeeded + upstream_error + timed_out + canceled + unknown)
);
CREATE TABLE usage_attempts (
 id text PRIMARY KEY,
 project_id text NOT NULL REFERENCES projects(id), key_id text NOT NULL REFERENCES api_keys(id),
 method text NOT NULL CHECK(method IN ('eth_chainId','eth_blockNumber','eth_getBalance','eth_getCode','eth_getTransactionReceipt')),
 cost_version integer NOT NULL DEFAULT 1 CHECK(cost_version=1),
 units bigint NOT NULL DEFAULT 1 CHECK(units=1),
 billable_units bigint NOT NULL DEFAULT 0 CHECK(billable_units=0),
 day date NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 outcome text NOT NULL DEFAULT 'pending' CHECK(outcome IN ('pending','succeeded','upstream_error','timed_out','canceled','unknown')),
 finished_at timestamptz,
 CHECK((outcome='pending') = (finished_at IS NULL))
);
CREATE INDEX usage_attempts_pending ON usage_attempts(created_at) WHERE outcome='pending';
CREATE INDEX usage_attempts_retention ON usage_attempts(created_at) WHERE outcome<>'pending';
CREATE INDEX usage_attempts_project_day ON usage_attempts(project_id,day);
CREATE INDEX usage_days_retention ON usage_days(day);
