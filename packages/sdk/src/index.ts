import { z } from "zod";

const StatusSchema = z.object({
  service: z.literal("api"),
  version: z.string(),
  mode: z.literal("scaffold"),
});
const PrincipalSchema = z.object({
  userId: z.string(),
  organizationId: z.string(),
  email: z.string(),
});
const ProjectSchema = z.object({
  id: z.string(),
  name: z.string(),
  chainId: z.literal(1),
  createdAt: z.string().datetime({ offset: true }),
});
const KeySchema = z.object({
  id: z.string(),
  projectId: z.string(),
  name: z.string(),
  prefix: z.string(),
  createdAt: z.string().datetime({ offset: true }),
  expiresAt: z.string().datetime({ offset: true }),
  revokedAt: z.string().datetime({ offset: true }).nullable(),
});
const IssuedKeySchema = z.object({
  key: KeySchema,
  secret: z.string().regex(/^infra_sk_[a-f0-9]{64}$/),
});
const KeyIdentitySchema = z.object({
  keyId: z.string(),
  projectId: z.string(),
  organizationId: z.string(),
  chainId: z.literal(1),
});
export type PlatformStatus = z.infer<typeof StatusSchema>;
export type Principal = z.infer<typeof PrincipalSchema>;
export type Project = z.infer<typeof ProjectSchema>;
export type APIKey = z.infer<typeof KeySchema>;
export type IssuedKey = z.infer<typeof IssuedKeySchema>;
export type KeyIdentity = z.infer<typeof KeyIdentitySchema>;

export const ProjectLimitsSchema = z.object({
  dailyUnits: z.number().int().min(0).max(100000),
  minuteRequests: z.number().int().min(0).max(600),
});
const Units = z.number().int().nonnegative().safe();
const ProjectUsageSchema = z.object({
  limits: ProjectLimitsSchema,
  usedUnits: Units,
  remainingUnits: Units,
  resetsAt: z.string().datetime({ offset: true }),
  days: z
    .array(
      z.object({
        day: z.string().regex(/^\d{4}-\d{2}-\d{2}$/),
        units: Units,
        pending: Units,
        succeeded: Units,
        upstreamError: Units,
        timedOut: Units,
        canceled: Units,
        unknown: Units,
      }),
    )
    .max(7),
});
export type ProjectLimits = z.infer<typeof ProjectLimitsSchema>;
export type ProjectUsage = z.infer<typeof ProjectUsageSchema>;

export class InfraError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
  ) {
    super(`Request failed (${status}): ${code}`);
    this.name = "InfraError";
  }
}

export class InfraClient {
  constructor(
    private readonly baseUrl: string,
    private readonly fetcher: typeof fetch = (...args) => fetch(...args),
  ) {}

  private async request<T>(
    path: string,
    schema: z.ZodType<T>,
    init: RequestInit = {},
    signal?: AbortSignal,
  ): Promise<T> {
    const timeout = AbortSignal.timeout(10_000);
    const response = await this.fetcher(`${this.baseUrl.replace(/\/$/, "")}${path}`, {
      credentials: "same-origin",
      ...init,
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    });
    if (!response.ok) {
      const body: unknown = await response.json().catch(() => null);
      const parsed = z.object({ code: z.string() }).safeParse(body);
      throw new InfraError(response.status, parsed.success ? parsed.data.code : "request_failed");
    }
    const data: unknown = await response.json();
    const parsed = schema.safeParse(data);
    if (!parsed.success) throw new Error("Invalid API response");
    return parsed.data;
  }

  status(signal?: AbortSignal): Promise<PlatformStatus> {
    return this.request("/v1/status", StatusSchema, {}, signal);
  }
  me(signal?: AbortSignal): Promise<Principal> {
    return this.request("/v1/me", PrincipalSchema, {}, signal);
  }
  projects(signal?: AbortSignal): Promise<{ projects: Project[] }> {
    return this.request("/v1/projects", z.object({ projects: z.array(ProjectSchema) }), {}, signal);
  }
  createProject(name: string): Promise<Project> {
    return this.request("/v1/projects", ProjectSchema, this.json({ name }));
  }
  keys(project: string, signal?: AbortSignal): Promise<{ keys: APIKey[] }> {
    return this.request(
      `/v1/projects/${encodeURIComponent(project)}/keys`,
      z.object({ keys: z.array(KeySchema) }),
      {},
      signal,
    );
  }
  issueKey(project: string, name: string): Promise<IssuedKey> {
    return this.request(
      `/v1/projects/${encodeURIComponent(project)}/keys`,
      IssuedKeySchema,
      this.json({ name }),
    );
  }
  rotateKey(project: string, key: string): Promise<IssuedKey> {
    return this.request(
      `/v1/projects/${encodeURIComponent(project)}/keys/${encodeURIComponent(key)}/rotate`,
      IssuedKeySchema,
      { method: "POST" },
    );
  }
  revokeKey(project: string, key: string): Promise<{ ok: true }> {
    return this.request(
      `/v1/projects/${encodeURIComponent(project)}/keys/${encodeURIComponent(key)}`,
      z.object({ ok: z.literal(true) }),
      { method: "DELETE" },
    );
  }
  checkKey(secret: string): Promise<KeyIdentity> {
    return this.request("/v1/key-check", KeyIdentitySchema, {
      headers: { Authorization: `Bearer ${secret}` },
    });
  }
  usage(project: string, signal?: AbortSignal): Promise<ProjectUsage> {
    return this.request(
      `/v1/projects/${encodeURIComponent(project)}/usage`,
      ProjectUsageSchema,
      {},
      signal,
    );
  }
  setLimits(project: string, limits: ProjectLimits): Promise<ProjectLimits> {
    const validated = ProjectLimitsSchema.parse(limits);
    return this.request(`/v1/projects/${encodeURIComponent(project)}/limits`, ProjectLimitsSchema, {
      ...this.json(validated),
      method: "PUT",
    });
  }
  logout(): Promise<{ ok: true }> {
    return this.request("/auth/logout", z.object({ ok: z.literal(true) }), { method: "POST" });
  }
  private json(body: unknown): RequestInit {
    return {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    };
  }
}

export { EthereumClient, EthereumError } from "./ethereum.js";
export type { BlockReference, TransactionReceipt } from "./ethereum.js";
