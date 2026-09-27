import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { InfraClient } from "@infra/sdk";
import "./style.css";

function App() {
  const [status, setStatus] = useState("Connecting…");
  useEffect(() => {
    const controller = new AbortController();
    void new InfraClient("").status(controller.signal)
      .then((result) => setStatus(`API connected · ${result.version}`))
      .catch(() => { if (!controller.signal.aborted) setStatus("API unavailable. Start it with make api."); });
    return () => controller.abort();
  }, []);
  return <main><span className="label">INFRA / DEVELOPMENT</span><h1>A foundation<br/>for what comes next.</h1>
    <p>Developer infrastructure for EVM applications.</p><section aria-live="polite"><span className="label">CONNECTION</span><h2>{status}</h2><p>This workspace connects the dashboard to the Go API through the TypeScript SDK.</p></section>
    <footer>Scaffold only · Billing, wallets, RPC forwarding, and durable jobs are not enabled.</footer></main>;
}
const root = document.getElementById("root");
if (!root) throw new Error("Missing application root");
createRoot(root).render(<StrictMode><App/></StrictMode>);
