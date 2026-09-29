import assert from "node:assert/strict";
import test from "node:test";
import { EthereumClient, EthereumError } from "../dist/index.js";

const key = `infra_sk_${"a".repeat(64)}`;
const address = `0x${"a".repeat(40)}`;
const hash = `0x${"b".repeat(64)}`;
function fixture(result, inspect = () => {}) {
  return new EthereumClient("http://127.0.0.1:8081", key, async (url, init) => {
    const req = JSON.parse(init.body);
    assert.equal(url, "http://127.0.0.1:8081/rpc");
    assert.equal(init.headers.Authorization, `Bearer ${key}`);
    assert.equal(init.credentials, "omit");
    assert.equal(init.redirect, "error");
    inspect(req);
    return Response.json({ jsonrpc: "2.0", id: req.id, result });
  });
}

test("Ethereum reads preserve exact integers and method parameters", async () => {
  assert.equal(await fixture("0x1").chainId(), 1);
  assert.equal(await fixture("0x20000000000001").blockNumber(), 9007199254740993n);
  assert.equal(
    await fixture("0xde0b6b3a7640000", (r) => {
      assert.equal(r.method, "eth_getBalance");
      assert.deepEqual(r.params, [address, "finalized"]);
    }).balance(address, "finalized"),
    1000000000000000000n,
  );
  assert.equal(await fixture("0x6000").code(address), "0x6000");
  assert.equal(await fixture(null).receipt(hash), null);
});

test("invalid gateway configuration and method parameters fail before fetch", async () => {
  for (const url of [
    "http://public.test",
    "https://user:secret@example.com",
    "https://example.com/?secret=1",
  ]) {
    assert.throws(() => new EthereumClient(url, key));
  }
  assert.throws(() => new EthereumClient("https://example.com", "bad-key"));
  const client = new EthereumClient("https://example.com", key, () => {
    assert.fail("fetch should not run");
  });
  assert.throws(() => client.balance("bad"));
  assert.throws(() => client.code(address, "0x00"));
  assert.throws(() => client.receipt("0x1"));
});

test("gateway errors, malformed responses and mismatched IDs fail without retry", async () => {
  for (const item of [
    { status: 401, body: { code: "invalid_api_key" }, code: "invalid_api_key" },
    { status: 503, body: { code: "gateway_busy" }, code: "gateway_busy" },
    {
      status: 502,
      body: { error: { code: -32001, message: "Upstream unavailable" } },
      code: -32001,
    },
    { status: 200, body: { id: "wrong", result: "0x1" }, code: "invalid_response" },
    { status: 200, body: { result: "0x2" }, code: "invalid_response" },
    {
      status: 200,
      body: { result: "0x1", error: { code: -1, message: "bad" } },
      code: "invalid_response",
    },
    { status: 200, body: {}, code: "invalid_response" },
  ]) {
    let calls = 0;
    const client = new EthereumClient("https://gateway.test", key, async (url, init) => {
      calls++;
      return Response.json(
        { jsonrpc: "2.0", id: JSON.parse(init.body).id, ...item.body },
        { status: item.status },
      );
    });
    await assert.rejects(
      client.chainId(),
      (err) => err instanceof EthereumError && err.code === item.code,
    );
    assert.equal(calls, 1);
  }
});

test("cancellation reaches fetch", async () => {
  const controller = new AbortController();
  controller.abort();
  const client = new EthereumClient("https://gateway.test", key, async (url, init) => {
    assert.equal(init.signal.aborted, true);
    init.signal.throwIfAborted();
  });
  await assert.rejects(client.chainId(controller.signal), { name: "AbortError" });
});
