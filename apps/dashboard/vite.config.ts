import { defineConfig, loadEnv } from "vite";
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "INFRA_");
  const api = env.INFRA_API_URL || "http://127.0.0.1:8080";
  return { server: { port: 5173, strictPort: true, proxy: { "/v1": api, "/auth": api } } };
});
