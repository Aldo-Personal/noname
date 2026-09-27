import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  InfraClient,
  InfraError,
  type Principal,
  type Project,
  type APIKey,
  type IssuedKey,
} from "@infra/sdk";
import "./style.css";

const client = new InfraClient("");
const date = (value: string) => new Date(value).toLocaleDateString();

function App() {
  const [me, setMe] = useState<Principal | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [projects, setProjects] = useState<Project[]>([]);
  const [selected, setSelected] = useState("");
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [keysLoading, setKeysLoading] = useState(false);
  const [projectName, setProjectName] = useState("");
  const [keyName, setKeyName] = useState("");
  const [issued, setIssued] = useState<IssuedKey | null>(null);
  const [testSecret, setTestSecret] = useState("");
  function report(err: unknown) {
    if (err instanceof InfraError && err.status === 401 && err.code !== "invalid_api_key") {
      setMe(null);
      setIssued(null);
      setKeys([]);
      setProjects([]);
      setSelected("");
      setTestSecret("");
      setError("Your session has ended. Sign in again.");
    } else if (err instanceof InfraError && err.code === "access_not_configured") {
      setError("Project access is not configured. Follow the local setup in the README.");
    } else {
      setError(err instanceof Error ? err.message : "Something went wrong.");
    }
  }
  useEffect(() => {
    const abort = new AbortController();
    void client
      .me(abort.signal)
      .then(async (user) => {
        const result = await client.projects(abort.signal);
        if (!abort.signal.aborted) {
          setMe(user);
          setProjects(result.projects);
          setSelected(result.projects[0]?.id ?? "");
        }
      })
      .catch((err) => {
        if (!abort.signal.aborted && !(err instanceof InfraError && err.status === 401))
          report(err);
      })
      .finally(() => {
        if (!abort.signal.aborted) setLoading(false);
      });
    return () => abort.abort();
  }, []);
  useEffect(() => {
    setKeys([]);
    setIssued(null);
    setTestSecret("");
    setNotice("");
    if (!selected || !me) {
      setKeysLoading(false);
      return;
    }
    const abort = new AbortController();
    setKeysLoading(true);
    void client
      .keys(selected, abort.signal)
      .then((result) => {
        if (!abort.signal.aborted) setKeys(result.keys);
      })
      .catch((err) => {
        if (!abort.signal.aborted) report(err);
      })
      .finally(() => {
        if (!abort.signal.aborted) setKeysLoading(false);
      });
    return () => abort.abort();
  }, [selected, me]);
  async function act(task: () => Promise<void>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await task();
    } catch (err) {
      report(err);
    } finally {
      setBusy(false);
    }
  }
  async function refreshKeys() {
    const result = await client.keys(selected);
    setKeys(result.keys);
  }
  const current = projects.find((project) => project.id === selected);

  return (
    <main>
      <header>
        <a className="brand" href="/">
          infra<span> / developer console</span>
        </a>
        <span className="badge">Ethereum · chain 1</span>
        {me && (
          <button
            className="secondary"
            disabled={busy}
            onClick={() =>
              void act(async () => {
                await client.logout();
                setMe(null);
                setProjects([]);
                setSelected("");
                setKeys([]);
                setIssued(null);
                setTestSecret("");
              })
            }
          >
            Sign out
          </button>
        )}
      </header>
      <div className="intro">
        <span className="eyebrow">YOUR APPLICATION STARTS HERE</span>
        <h1>Build on Ethereum.</h1>
        <p>Create a project. Give your backend a key. Keep access under your control.</p>
      </div>
      {error && (
        <div className="message error" role="alert">
          {error}
        </div>
      )}
      {notice && (
        <div className="message" role="status">
          {notice}
        </div>
      )}
      {loading ? (
        <p role="status">Loading your workspace…</p>
      ) : !me ? (
        <section className="signin">
          <span className="eyebrow">PROJECT ACCESS</span>
          <h2>Your workspace, one sign-in away.</h2>
          <p>Sign in to manage Ethereum projects and server API keys.</p>
          <a className="button" href="/auth/login">
            Sign in
          </a>
          <p className="muted">
            Local development uses Keycloak. See the README for the development account.
          </p>
        </section>
      ) : (
        <>
          <div className="workspace">
            <span>{me.email || "Personal workspace"}</span>
            <span className="muted">Personal organization · {me.organizationId.slice(0, 8)}</span>
          </div>
          <div className="grid">
            <aside className="panel">
              <h2>
                Projects <span className="count">{projects.length}</span>
              </h2>
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  void act(async () => {
                    const project = await client.createProject(projectName.trim());
                    setProjects((items) => [project, ...items]);
                    setSelected(project.id);
                    setProjectName("");
                  });
                }}
              >
                <label htmlFor="project-name">Project name</label>
                <input
                  id="project-name"
                  value={projectName}
                  onChange={(event) => setProjectName(event.target.value)}
                  maxLength={80}
                  required
                  placeholder="My Ethereum app"
                  disabled={busy}
                />
                <button disabled={busy || !projectName.trim()}>Create project</button>
              </form>
              <nav aria-label="Projects">
                {projects.length === 0 && (
                  <p className="muted">No projects yet. Create your first one above.</p>
                )}
                {projects.map((project) => (
                  <button
                    className={`project ${selected === project.id ? "selected" : ""}`}
                    key={project.id}
                    disabled={busy}
                    onClick={() => {
                      setError("");
                      setSelected(project.id);
                    }}
                  >
                    <strong>{project.name}</strong>
                    <span>Ethereum mainnet</span>
                  </button>
                ))}
              </nav>
            </aside>
            <section className="panel details">
              {!current ? (
                <div className="empty">
                  <h2>A home for your application.</h2>
                  <p>Create or select a project to manage its API keys.</p>
                </div>
              ) : (
                <>
                  <div className="section-title">
                    <div>
                      <span className="eyebrow">ETHEREUM MAINNET</span>
                      <h2>{current.name}</h2>
                    </div>
                    <span className="badge">Chain ID 1</span>
                  </div>
                  <p className="mono muted">{current.id}</p>
                  <h3>Server API keys</h3>
                  <p className="muted">
                    Keys expire after 90 days. Keep them on your server; never ship them in browser
                    code.
                  </p>
                  <form
                    className="inline"
                    onSubmit={(event) => {
                      event.preventDefault();
                      void act(async () => {
                        const result = await client.issueKey(selected, keyName.trim());
                        setIssued(result);
                        setKeyName("");
                        await refreshKeys();
                      });
                    }}
                  >
                    <div>
                      <label htmlFor="key-name">Key name</label>
                      <input
                        id="key-name"
                        value={keyName}
                        onChange={(event) => setKeyName(event.target.value)}
                        maxLength={80}
                        required
                        placeholder="Development backend"
                        disabled={busy}
                      />
                    </div>
                    <button disabled={busy || keysLoading || !keyName.trim()}>Generate key</button>
                  </form>
                  {issued && (
                    <div className="secret" role="status">
                      <strong>Save this key now. It will not be shown again.</strong>
                      <code>{issued.secret}</code>
                      <div className="actions">
                        <button
                          className="secondary"
                          disabled={busy}
                          onClick={() =>
                            void act(async () => {
                              await navigator.clipboard.writeText(issued.secret);
                              setNotice("Key copied to clipboard.");
                            })
                          }
                        >
                          Copy key
                        </button>
                        <button
                          className="secondary"
                          disabled={busy}
                          onClick={() =>
                            void act(async () => {
                              const result = await client.checkKey(issued.secret);
                              setNotice(
                                `Authenticated for Ethereum project ${result.projectId.slice(0, 8)}.`,
                              );
                            })
                          }
                        >
                          Test key
                        </button>
                        <button
                          className="secondary"
                          disabled={busy}
                          onClick={() => setIssued(null)}
                        >
                          Dismiss
                        </button>
                      </div>
                    </div>
                  )}
                  {keysLoading ? (
                    <p role="status">Loading keys…</p>
                  ) : (
                    <ul className="keys">
                      {keys.map((key) => {
                        const active =
                          !key.revokedAt && new Date(key.expiresAt).getTime() > Date.now();
                        return (
                          <li key={key.id}>
                            <div>
                              <strong>{key.name}</strong>
                              <code>{key.prefix}…</code>
                              <small>
                                {key.revokedAt
                                  ? "Revoked"
                                  : active
                                    ? `Expires ${date(key.expiresAt)}`
                                    : "Expired"}
                              </small>
                            </div>
                            <div className="actions">
                              <button
                                className="secondary"
                                disabled={busy || !active}
                                onClick={() => {
                                  if (
                                    window.confirm(
                                      "Rotate this key? The old key stops working immediately.",
                                    )
                                  )
                                    void act(async () => {
                                      setIssued(null);
                                      setIssued(await client.rotateKey(selected, key.id));
                                      await refreshKeys();
                                    });
                                }}
                              >
                                Rotate
                              </button>
                              <button
                                className="danger"
                                disabled={busy || !active}
                                onClick={() => {
                                  if (
                                    window.confirm(
                                      "Revoke this key? Applications using it will lose access.",
                                    )
                                  )
                                    void act(async () => {
                                      await client.revokeKey(selected, key.id);
                                      if (issued?.key.id === key.id) setIssued(null);
                                      await refreshKeys();
                                      setNotice("Key revoked.");
                                    });
                                }}
                              >
                                Revoke
                              </button>
                            </div>
                          </li>
                        );
                      })}
                      {keys.length === 0 && <li className="muted">No keys yet.</li>}
                    </ul>
                  )}
                  <details>
                    <summary>Check an existing key</summary>
                    <form
                      className="inline"
                      onSubmit={(event) => {
                        event.preventDefault();
                        void act(async () => {
                          const secret = testSecret;
                          setTestSecret("");
                          const identity = await client.checkKey(secret);
                          setNotice(
                            `Key authenticated for project ${identity.projectId.slice(0, 8)}.`,
                          );
                        });
                      }}
                    >
                      <div>
                        <label htmlFor="test-key">API key</label>
                        <input
                          id="test-key"
                          type="password"
                          autoComplete="off"
                          value={testSecret}
                          onChange={(event) => setTestSecret(event.target.value)}
                          disabled={busy}
                          required
                        />
                      </div>
                      <button disabled={busy || !testSecret}>Check key</button>
                    </form>
                  </details>
                </>
              )}
            </section>
          </div>
        </>
      )}
      <footer>
        Development access milestone · RPC forwarding, transactions, and billing are not enabled.
      </footer>
    </main>
  );
}
const root = document.getElementById("root");
if (!root) throw new Error("Missing application root");
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
