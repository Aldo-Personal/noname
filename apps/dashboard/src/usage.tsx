import { useEffect, useState } from "react";
import { InfraClient, type ProjectUsage } from "@infra/sdk";

const client = new InfraClient("");

export function UsagePanel({ project }: { project: string }) {
  const [usage, setUsage] = useState<ProjectUsage | null>(null);
  const [daily, setDaily] = useState(10000);
  const [minute, setMinute] = useState(60);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const abort = new AbortController();
    setBusy(true);
    setError("");
    void client
      .usage(project, abort.signal)
      .then((value) => {
        if (abort.signal.aborted) return;
        setUsage(value);
        setDaily(value.limits.dailyUnits);
        setMinute(value.limits.minuteRequests);
      })
      .catch((err: unknown) => {
        if (!abort.signal.aborted)
          setError(err instanceof Error ? err.message : "Could not load usage.");
      })
      .finally(() => {
        if (!abort.signal.aborted) setBusy(false);
      });
    return () => abort.abort();
  }, [project, revision]);

  return (
    <section className="usage" aria-label="Project usage and limits">
      <div className="section-title">
        <h3>Usage and limits</h3>
        <button
          className="secondary"
          disabled={busy}
          onClick={() => setRevision((value) => value + 1)}
        >
          Refresh usage
        </button>
      </div>
      {error && (
        <p className="message error" role="alert">
          {error}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {busy && <p role="status">Loading usage…</p>}
      {usage && (
        <>
          <p>
            <strong>{usage.usedUnits.toLocaleString()}</strong> /{" "}
            {usage.limits.dailyUnits.toLocaleString()} units used today ·{" "}
            {usage.remainingUnits.toLocaleString()} remaining
          </p>
          <p className="muted">
            Resets {new Date(usage.resetsAt).toLocaleString()}. Each admitted read uses one unit,
            including failed or interrupted requests. No money is charged.
          </p>
          <form
            className="inline"
            onSubmit={(event) => {
              event.preventDefault();
              setBusy(true);
              setError("");
              setNotice("");
              void client
                .setLimits(project, { dailyUnits: daily, minuteRequests: minute })
                .then(() => {
                  setNotice("Limits saved. Existing usage is unchanged.");
                  setRevision((value) => value + 1);
                })
                .catch((err: unknown) => {
                  setError(err instanceof Error ? err.message : "Could not save limits.");
                  setBusy(false);
                });
            }}
          >
            <div>
              <label htmlFor="daily-units">Daily units</label>
              <input
                id="daily-units"
                type="number"
                min="0"
                max="100000"
                step="1"
                required
                value={daily}
                disabled={busy}
                onChange={(event) => setDaily(event.target.valueAsNumber)}
              />
            </div>
            <div>
              <label htmlFor="minute-requests">Requests per minute</label>
              <input
                id="minute-requests"
                type="number"
                min="0"
                max="600"
                step="1"
                required
                value={minute}
                disabled={busy}
                onChange={(event) => setMinute(event.target.valueAsNumber)}
              />
            </div>
            <button disabled={busy}>Save limits</button>
          </form>
          <p className="muted">
            Set either limit to zero to pause RPC access. Development maximum: 100,000 units/day and
            600 requests/minute.
          </p>
          <details>
            <summary>Last seven days</summary>
            {usage.days.length === 0 ? (
              <p>No RPC usage yet.</p>
            ) : (
              <ul className="keys">
                {usage.days.map((day) => (
                  <li key={day.day}>
                    <div>
                      <strong>
                        {day.day} · {day.units.toLocaleString()} units
                      </strong>
                      <small>
                        {day.succeeded} succeeded · {day.upstreamError} provider errors ·{" "}
                        {day.timedOut} timed out · {day.canceled} canceled · {day.pending} pending ·{" "}
                        {day.unknown} unknown
                      </small>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </details>
        </>
      )}
    </section>
  );
}
