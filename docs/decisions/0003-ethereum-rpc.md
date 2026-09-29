# ADR 0003: A bounded read-only Ethereum gateway

Status: accepted for development. Tracking issue: #9. Extends ADR 0002's key usage.

Use the standard Go HTTP client against one operator-configured Ethereum JSON-RPC
provider. No provider-specific dependency or chain abstraction is needed for five
read methods. Ethereum mainnet is verified at startup with eth_chainId. Provider
commercial terms, capacity and a live upstream credential remain operator prerequisites.

Authenticate each request with the existing PostgreSQL key store. No authorization
cache: committed revocation applies to subsequent requests, not already-authorized
in-flight work. PostgreSQL failure fails closed. Gateway configuration needs the
database and provider URL; OIDC runs only in the management API.

Reject methods outside the five-method allowlist, malformed parameters, batches and
notifications. This is an intentional subset of JSON-RPC 2.0, not a general Ethereum
endpoint. Rebuild outbound requests, drop incoming headers, do not follow redirects,
and hide provider error payloads. API keys authorize the stored project's chain, never
a caller-supplied project, tenant or destination. Server SDK keys must stay server-side.

Bound payloads, deadlines and in-flight work. A local 32-request semaphore is a process
safety limit, not tenant quotas or a measured capacity claim. No retries or failover:
those need attempt accounting before enabling them. The next milestone (#10) supplies
distributed project limits and durable usage accounting. Production remains blocked.

Configured readiness checks PostgreSQL. Startup checks the provider's chain; runtime
provider failures return sanitized errors. Readiness is not continuous upstream health
monitoring. Observability and provider health policy remain production prerequisites.

Response envelopes, quantities and bytecode are validated. Receipt objects remain
fork-extensible; this gateway does not independently verify chain data or finality.
No schema migration, additional dependency, signing or transaction submission is added.

References: [JSON-RPC 2.0](https://www.jsonrpc.org/specification) and
[Ethereum JSON-RPC](https://ethereum.org/en/developers/docs/apis/json-rpc/).
