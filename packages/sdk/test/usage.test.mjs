import assert from "node:assert/strict";
import test from "node:test";
import { InfraClient, InfraError } from "../dist/index.js";

const summary = {
  limits: { dailyUnits: 20, minuteRequests: 5 },
  usedUnits: 7,
  remainingUnits: 13,
  resetsAt: "2026-10-01T00:00:00Z",
  days: [],
};
test("usage summary is validated and project paths are encoded", async () => {
  const client = new InfraClient("", async (path) => {
    assert.equal(path, "/v1/projects/a%2Fb/usage");
    return Response.json(summary);
  });
  assert.equal((await client.usage("a/b")).remainingUnits, 13);
  const invalid = new InfraClient("", async () => Response.json({ ...summary, usedUnits: -1 }));
  await assert.rejects(invalid.usage("p"), /Invalid API response/);
});
test("limits require valid integers and use a session-authenticated PUT", async () => {
  const client = new InfraClient("", async (path, init) => {
    assert.equal(path, "/v1/projects/p/limits");
    assert.equal(init.method, "PUT");
    assert.equal(init.credentials, "same-origin");
    assert.equal(init.headers.Authorization, undefined);
    assert.deepEqual(JSON.parse(init.body), summary.limits);
    return Response.json(summary.limits);
  });
  assert.deepEqual(await client.setLimits("p", summary.limits), summary.limits);
  assert.throws(() => client.setLimits("p", { dailyUnits: 100001, minuteRequests: 1 }));
  const denied = new InfraClient("", async () =>
    Response.json({ code: "not_found" }, { status: 404 }),
  );
  await assert.rejects(
    denied.usage("other-project"),
    (err) => err instanceof InfraError && err.status === 404,
  );
});
