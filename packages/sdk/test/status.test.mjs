import test from "node:test";
import assert from "node:assert/strict";
import { InfraClient } from "../dist/index.js";

test("validates the API contract and normalizes trailing slash", async () => {
  const client = new InfraClient("https://example.test/", async (url) => {
    assert.equal(url, "https://example.test/v1/status");
    return Response.json({service:"api", version:"0.1.0", mode:"scaffold"});
  });
  assert.equal((await client.status()).service, "api");
});
test("rejects malformed responses", async () => {
  const client = new InfraClient("", async () => Response.json({service:"api"}));
  await assert.rejects(client.status(), /Invalid status/);
});
test("rejects upstream failures", async () => {
  const client = new InfraClient("", async () => new Response(null, {status:503}));
  await assert.rejects(client.status(), /503/);
});
