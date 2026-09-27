import { spawn, execFileSync } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { once } from "node:events";
import { createServer } from "node:net";
import assert from "node:assert/strict";
import { InfraClient } from "../packages/sdk/dist/index.js";

const directory = await mkdtemp(join(tmpdir(), "infra-smoke-"));
try {
  for (const service of ["api", "gateway", "worker"]) {
    const binary = join(directory, service);
    execFileSync("go", ["build", "-o", binary, `./cmd/${service}`]);
    const socket = createServer();
    socket.listen(0, "127.0.0.1");
    await once(socket, "listening");
    const port = socket.address().port;
    await new Promise((resolve, reject) => socket.close((err) => (err ? reject(err) : resolve())));
    const base = `http://127.0.0.1:${port}`;
    const child = spawn(binary, [], {
      env: {
        ...process.env,
        APP_ENV: "test",
        DATABASE_URL: "",
        OIDC_ISSUER: "",
        OIDC_CLIENT_ID: "",
        PUBLIC_ORIGIN: "",
        HTTP_ADDR: `127.0.0.1:${port}`,
      },
      stdio: "ignore",
    });
    const exited = once(child, "exit");
    try {
      let healthy = false;
      for (let attempt = 0; attempt < 100; attempt++) {
        try {
          healthy = (await fetch(`${base}/healthz`, { signal: AbortSignal.timeout(500) })).ok;
        } catch {}
        if (healthy) break;
        if (child.exitCode !== null) throw new Error(`${service} exited during startup`);
        await new Promise((resolve) => setTimeout(resolve, 50));
      }
      assert.ok(healthy, `${service} failed to start`);
      assert.equal((await fetch(`${base}/readyz`)).status, service === "worker" ? 503 : 200);
      if (service === "api") assert.equal((await new InfraClient(base).status()).service, "api");
      if (service === "gateway")
        assert.equal((await fetch(`${base}/rpc`, { method: "POST" })).status, 503);
    } finally {
      child.kill("SIGTERM");
      const timer = setTimeout(() => child.kill("SIGKILL"), 12_000);
      const [code] = await exited;
      clearTimeout(timer);
      assert.equal(code, 0, `${service} failed graceful shutdown`);
    }
    console.log(`PASS ${service}: HTTP behavior and graceful shutdown`);
  }
} finally {
  await rm(directory, { recursive: true, force: true });
}
