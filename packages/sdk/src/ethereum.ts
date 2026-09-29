import { z } from "zod";

const Quantity = z.string().regex(/^0x(0|[1-9a-fA-F][0-9a-fA-F]{0,63})$/);
const Data = z.string().regex(/^0x([0-9a-fA-F]{2})*$/);
const Address = z.string().regex(/^0x[0-9a-fA-F]{40}$/);
const Hash = z.string().regex(/^0x[0-9a-fA-F]{64}$/);
const Block = z.union([z.enum(["latest", "earliest", "safe", "finalized", "pending"]), Quantity]);
const Receipt = z
  .object({
    transactionHash: Hash,
    transactionIndex: Quantity,
    blockHash: Hash,
    blockNumber: Quantity,
    from: Address,
    to: Address.nullable(),
    contractAddress: Address.nullable(),
    cumulativeGasUsed: Quantity,
    gasUsed: Quantity,
    logs: z.array(z.record(z.string(), z.unknown())),
    status: Quantity.optional(),
    root: Hash.optional(),
  })
  .passthrough();

export type BlockReference =
  "latest" | "earliest" | "safe" | "finalized" | "pending" | `0x${string}`;
export type TransactionReceipt = z.infer<typeof Receipt>;

export class EthereumError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: number | string,
  ) {
    super(`Ethereum request failed (${status}): ${code}`);
    this.name = "EthereumError";
  }
}

/** Server-side only: project keys must never be shipped to a browser bundle. */
export class EthereumClient {
  private readonly endpoint: string;
  constructor(
    gatewayUrl: string,
    private readonly apiKey: string,
    private readonly fetcher: typeof fetch = (...args) => fetch(...args),
  ) {
    if (typeof window !== "undefined") throw new Error("EthereumClient requires a server runtime");
    const url = new URL(gatewayUrl);
    const local = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname);
    if (
      (url.protocol !== "https:" && !(url.protocol === "http:" && local)) ||
      url.username ||
      url.password ||
      url.search ||
      url.hash
    ) {
      throw new Error("Invalid gateway URL");
    }
    if (!/^infra_sk_[a-f0-9]{64}$/.test(apiKey)) throw new Error("Invalid project API key");
    this.endpoint = `${gatewayUrl.replace(/\/$/, "")}/rpc`;
  }

  private async call<T>(
    method: string,
    params: unknown[],
    schema: z.ZodType<T>,
    signal?: AbortSignal,
  ): Promise<T> {
    const id = crypto.randomUUID();
    const timeout = AbortSignal.timeout(12_000);
    const response = await this.fetcher(this.endpoint, {
      method: "POST",
      credentials: "omit",
      redirect: "error",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${this.apiKey}` },
      body: JSON.stringify({ jsonrpc: "2.0", id, method, params }),
      signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    });
    const body: unknown = await response.json().catch(() => null);
    if (!response.ok) {
      const transport = z.object({ code: z.string() }).safeParse(body);
      if (transport.success) throw new EthereumError(response.status, transport.data.code);
    }
    const envelope = z
      .object({
        jsonrpc: z.literal("2.0"),
        id: z.literal(id),
        result: z.unknown().optional(),
        error: z.object({ code: z.number().int(), message: z.string() }).optional(),
      })
      .safeParse(body);
    if (!envelope.success) throw new EthereumError(response.status, "invalid_response");
    const value = envelope.data;
    if (value.error) {
      if (Object.hasOwn(value, "result"))
        throw new EthereumError(response.status, "invalid_response");
      throw new EthereumError(response.status, value.error.code);
    }
    if (!response.ok || !Object.hasOwn(value, "result"))
      throw new EthereumError(response.status, "invalid_response");
    const parsed = schema.safeParse(value.result);
    if (!parsed.success) throw new EthereumError(response.status, "invalid_response");
    return parsed.data;
  }

  chainId(signal?: AbortSignal): Promise<1> {
    return this.call(
      "eth_chainId",
      [],
      z.literal("0x1").transform(() => 1 as const),
      signal,
    );
  }
  blockNumber(signal?: AbortSignal): Promise<bigint> {
    return this.call(
      "eth_blockNumber",
      [],
      Quantity.transform((value) => BigInt(value)),
      signal,
    );
  }
  balance(
    address: string,
    block: BlockReference = "latest",
    signal?: AbortSignal,
  ): Promise<bigint> {
    return this.call(
      "eth_getBalance",
      [Address.parse(address), Block.parse(block)],
      Quantity.transform((value) => BigInt(value)),
      signal,
    );
  }
  code(address: string, block: BlockReference = "latest", signal?: AbortSignal): Promise<string> {
    return this.call("eth_getCode", [Address.parse(address), Block.parse(block)], Data, signal);
  }
  receipt(transactionHash: string, signal?: AbortSignal): Promise<TransactionReceipt | null> {
    return this.call(
      "eth_getTransactionReceipt",
      [Hash.parse(transactionHash)],
      Receipt.nullable(),
      signal,
    );
  }
}
