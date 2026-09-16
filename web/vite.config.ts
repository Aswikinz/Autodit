import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { readFileSync } from "node:fs";
import { synchronousJdmChanges } from "./src/lib/jdmTransform";
const jdmVersion = (
  JSON.parse(
    readFileSync(
      new URL(
        "./node_modules/@gorules/jdm-editor/package.json",
        import.meta.url,
      ),
      "utf8",
    ),
  ) as { version: string }
).version;
export default defineConfig({
  plugins: [
    {
      name: "autodit-jdm-controlled-values",
      enforce: "pre",
      transform(source, id) {
        if (
          !id
            .replaceAll("\\", "/")
            .split("?")[0]
            ?.endsWith("/@gorules/jdm-editor/dist/index.js")
        )
          return;
        return { code: synchronousJdmChanges(source, jdmVersion), map: null };
      },
    },
    react(),
  ],
  optimizeDeps: { exclude: ["@gorules/jdm-editor"] },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/auth": "http://localhost:8080",
    },
  },
  build: { chunkSizeWarningLimit: 1800 },
});
