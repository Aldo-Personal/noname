import test from "node:test";
import assert from "node:assert/strict";
import { InfraClient, InfraError } from "../dist/index.js";

test("project creation sends JSON and validates Ethereum chain ID", async () => {
  const client = new InfraClient("", async (url, init) => {
    assert.equal(url, "/v1/projects");
    assert.equal(init.method, "POST");
    assert.equal(init.credentials, "same-origin");
    assert.deepEqual(JSON.parse(init.body), { name: "Example" });
    return Response.json({
      id: "p",
      name: "Example",
      chainId: 1,
      createdAt: "2026-09-27T12:00:00Z",
    });
  });
  assert.equal((await client.createProject("Example")).chainId, 1);
});
test("rejects unsupported chain responses", async () => {
  const client = new InfraClient("", async () =>
    Response.json({
      projects: [{ id: "p", name: "Other", chainId: 137, createdAt: "2026-09-27T12:00:00Z" }],
    }),
  );
  await assert.rejects(client.projects(), /Invalid API response/);
});
test("key check sends secret only in Authorization and surfaces revocation", async () => {
  const secret = "infra_sk_" + "a".repeat(64);
  const client = new InfraClient("", async (url, init) => {
    assert.equal(url.includes(secret), false);
    assert.equal(init.headers.Authorization, `Bearer ${secret}`);
    return Response.json({ code: "invalid_api_key" }, { status: 401 });
  });
  await assert.rejects(
    client.checkKey(secret),
    (err) => err instanceof InfraError && err.status === 401,
  );
});
test("encodes project and key path segments", async () => {
  const client = new InfraClient("", async (url, init) => {
    assert.equal(url, "/v1/projects/a%2Fb/keys/c%2Fd");
    assert.equal(init.method, "DELETE");
    return Response.json({ ok: true });
  });
  await client.revokeKey("a/b", "c/d");
});

test("default fetch does not bind the browser API to the client", async (t) => {
  t.mock.method(globalThis, "fetch", function () {
    assert.equal(this, undefined);
    return Promise.resolve(Response.json({ service: "api", version: "0.1.0", mode: "scaffold" }));
  });
  assert.equal((await new InfraClient("").status()).service, "api");
});
