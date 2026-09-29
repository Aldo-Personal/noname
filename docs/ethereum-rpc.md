# Ethereum RPC gateway

Issue #9 adds authenticated, read-only Ethereum mainnet access for development.
The gateway is separate from the management API. Keys issued by the console work
at `POST http://127.0.0.1:8081/rpc` with `Authorization: Bearer <project-key>`.

## Start

After following README setup and applying migrations, export DATABASE_URL and an
operator-owned ETHEREUM_RPC_URL, then run `make gateway`. Keep provider credentials
in your local ignored .env or a secret manager. HTTPS is required except loopback
HTTP for local fixtures/nodes. Credentials in the URL path/query are supported;
userinfo and fragments are rejected. Do not expose the provider URL to customers.
Choose a provider whose terms allow your intended service.

When ETHEREUM_RPC_URL is absent, RPC stays disabled (503), even if DATABASE_URL is
set for the API. When present, DATABASE_URL is required and startup verifies applied
migrations and upstream chain ID 1. Wrong chain/unreachable provider prevents startup.
OIDC environment variables are not needed for the gateway. Configuration changes need
a process restart. No live upstream is supplied or contacted by automated tests.

## Supported contract

Use Content-Type: application/json, one JSON object with jsonrpc "2.0", a required
string or integer id (at most 128 encoded bytes), method and an array of params.
No-argument methods may omit params. IDs are preserved without float conversion.
Null/fractional IDs, notifications, batch arrays, unknown envelope fields, duplicate
envelope fields and trailing JSON are rejected. Compressed request bodies are rejected.

| Method | Parameters | Result |
| --- | --- | --- |
| eth_chainId | [] | "0x1" |
| eth_blockNumber | [] | Hex quantity |
| eth_getBalance | [address, block] | Wei as a hex quantity |
| eth_getCode | [address, block] | Hex bytecode |
| eth_getTransactionReceipt | [transaction hash] | Receipt object or null |

Addresses are 20-byte hex strings; transaction hashes are 32-byte hex strings.
Block may be latest, earliest, safe, finalized, pending, or a canonical hex quantity
of up to 256 bits. EIP-1898 block objects are not supported. Historical availability
depends on the selected provider. No eth_call, logs, writes, debug/admin, WebSocket
subscriptions, caching, retries, or failover in this milestone.

Example body:

```json
{"jsonrpc":"2.0","id":"balance-1","method":"eth_getBalance","params":["0x0000000000000000000000000000000000000000","latest"]}
```

## Server TypeScript SDK

```ts
import { EthereumClient } from "@infra/sdk";

const ethereum = new EthereumClient(
  "http://127.0.0.1:8081",
  process.env.INFRA_API_KEY!,
);
const block = await ethereum.blockNumber(); // bigint
const wei = await ethereum.balance("0x0000000000000000000000000000000000000000");
console.log(block.toString(), wei.toString());
```

Other methods: chainId(), code(address, block?), receipt(hash). All accept an optional
AbortSignal last. Receipts retain hex fields; convert quantities with BigInt, not Number.
EthereumError exposes status and code. The SDK validates responses, rejects redirects,
and never retries. Its 12-second deadline exceeds the gateway's 10-second operation
deadline. Do not import this client with a project secret into dashboard/browser code.
The constructor rejects browser runtimes, but runtime checks cannot protect bundled secrets.

## Limits and failures

Per request: 16 KiB input, 2 MiB provider response, 32 KiB provider response headers.
Per process: at most 32 admitted RPC requests, including authorization; excess requests
receive 503 gateway_busy and Retry-After: 1. Provider request timeout is 8 seconds;
the overall authorization/provider context is 10 seconds. HTTP reads/writes additionally
have the shared server's 15-second deadlines. These limits are fixed defaults, not an SLO.

| HTTP | Body / meaning |
| --- | --- |
| 200 | JSON-RPC result with original id |
| 400 | JSON-RPC error: -32700 parse, -32600 envelope, -32601 method, -32602 params; unreadable HTTP body uses code invalid_body |
| 401 | code invalid_api_key (missing, invalid, expired or revoked) |
| 403 | code unsupported_chain |
| 405 | Non-POST method |
| 413 / 415 | code request_too_large / unsupported_media_type |
| 429 | code project_limit_exceeded; Retry-After identifies the window reset |
| 502 | JSON-RPC -32001 Upstream unavailable (includes provider HTTP/RPC errors and invalid/oversized results) |
| 503 | code not_configured, gateway_busy, accounting_unavailable or authorization_unavailable |
| 504 | JSON-RPC -32002 Upstream timeout |

Do not depend on provider-specific error text: it is deliberately removed along with
provider response headers. Responses include a gateway X-Request-ID; secrets and RPC
bodies are not logged. `/healthz` is process liveness; configured `/readyz` checks the
database. Provider health is checked at startup and failures surface on RPC calls.
Unconfigured gateway readiness remains scaffold HTTP readiness, not RPC availability.

## Verification and recovery

`make check` runs protocol, adapter, HTTP, SDK and process smoke tests. Supply
TEST_DATABASE_URL for real PostgreSQL lifecycle/failure integration tests; CI does so.
Fake providers cover wrong networks, redirects, malformed/oversized responses, deadlines,
cancelled requests and credential isolation. `go test ./internal/ethereum -fuzz=FuzzParse
-fuzztime=10s` exercises the parser (run on one line).

After configuring a real provider, verify chainId/blockNumber and a known balance,
then rotate/revoke a disposable project key and verify it receives 401. That live smoke
test is an operator prerequisite; fixture tests do not measure production capacity.

Disable forwarding by removing ETHEREUM_RPC_URL and restarting the gateway. For rollback, keep applied migrations and disable forwarding before reverting accounting
binaries; see usage-accounting.md.
Keep the management API running for key revocation. Shared project quotas and durable usage are implemented in #10; see usage-accounting.md.
Do not enable public production before the operations gate is satisfied.
