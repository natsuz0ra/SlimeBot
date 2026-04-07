import { defineConfig } from "tsup";

export default defineConfig({
  entry: ["src/index.tsx"],
  format: ["esm"],
  target: "node20",
  bundle: true,
  outDir: "dist",
  clean: true,
  // ESM 不支持 shebang，我们需要在 entry 中处理
  banner: {},
});
