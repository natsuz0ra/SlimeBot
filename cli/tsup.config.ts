import { defineConfig } from "tsup";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  entry: ["src/index.tsx"],
  tsconfig: resolve(__dirname, "tsconfig.json"),
  format: ["esm"],
  target: "node20",
  bundle: true,
  external: ["@slimebot/color-diff-native"],
  noExternal: [/^(?!@slimebot\/color-diff-native$).+/],
  shims: true,
  outDir: "dist",
  clean: true,
  // ESM bundles cannot carry shebang; handle entry/bootstrap in app code.
  banner: {
    js: 'import { createRequire as __slimebotCreateRequire } from "node:module"; const require = __slimebotCreateRequire(import.meta.url);',
  },
  esbuildOptions(options) {
    options.alias = {
      ...options.alias,
      "react": resolve(__dirname, "node_modules/react/index.js"),
      "react/jsx-runtime": resolve(__dirname, "node_modules/react/jsx-runtime.js"),
      "react/compiler-runtime": resolve(__dirname, "node_modules/react/compiler-runtime.js"),
      "react-reconciler": resolve(__dirname, "node_modules/react-reconciler/index.js"),
      "react-reconciler/constants.js": resolve(__dirname, "node_modules/react-reconciler/constants.js"),
    };
  },
});
