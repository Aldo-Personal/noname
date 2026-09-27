export interface PlatformStatus {
  service: "api";
  version: string;
  mode: "scaffold";
}

export class InfraClient {
  constructor(private readonly baseUrl: string, private readonly fetcher: typeof fetch = fetch) {}

  async status(signal?: AbortSignal): Promise<PlatformStatus> {
    const timeout = AbortSignal.timeout(10_000);
    const response = await this.fetcher(`${this.baseUrl.replace(/\/$/, "")}/v1/status`, {
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    });
    if (!response.ok) throw new Error(`Status request failed (${response.status})`);
    const data: unknown = await response.json();
    if (typeof data !== "object" || data === null || !("service" in data) ||
        data.service !== "api" || !("version" in data) || typeof data.version !== "string" ||
        !("mode" in data) || data.mode !== "scaffold") {
      throw new Error("Invalid status response");
    }
    return { service: data.service, version: data.version, mode: data.mode };
  }
}
