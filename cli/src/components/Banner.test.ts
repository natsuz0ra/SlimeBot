import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { Banner } from "./Banner";
import { CLI_THEME } from "../utils/theme.js";

function collectText(node: unknown): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (!React.isValidElement(node)) return "";
  const props = node.props as { children?: unknown };
  const children = props.children;
  if (Array.isArray(children)) return children.map(collectText).join("");
  return collectText(children);
}

function collectBackgroundColors(node: unknown): string[] {
  if (!React.isValidElement(node)) return [];
  const props = node.props as { backgroundColor?: string; children?: unknown };
  const colors = props.backgroundColor ? [props.backgroundColor] : [];
  const children = props.children;
  if (Array.isArray(children)) {
    return colors.concat(...children.map(collectBackgroundColors));
  }
  return colors.concat(collectBackgroundColors(children));
}

test("Banner omits update marker when no update is available", () => {
  const text = collectText(
    Banner({ version: "1.26.1", modelName: "gpt", cwd: "/repo" }),
  );

  assert.match(text, /1\.26\.1/);
  assert.doesNotMatch(text, /\[new\]/);
});

test("Banner shows a compact update marker without the latest version", () => {
  const text = collectText(
    Banner({
      version: "1.26.1",
      modelName: "gpt",
      cwd: "/repo",
      updateAvailable: true,
    }),
  );

  assert.match(text, /1\.26\.1 \[new\]/);
  assert.doesNotMatch(text, /v1\.26\.2/);
});

test("Banner renders seamless background-colored pixel Slime logo and optional session name", async () => {
  const element = Banner({
    version: "1.36.0",
    modelName: "claude",
    cwd: "/repo",
    sessionName: "Fix login",
  });
  const text = collectText(element);
  const bgColors = collectBackgroundColors(element);

  assert.match(text, /Fix login/);
  assert.doesNotMatch(text, /█/);
  assert.ok(bgColors.includes(CLI_THEME.logo));
  assert.ok(bgColors.includes(CLI_THEME.logoEye));

  const { renderToScreen } = await import(new URL("../../packages/ink/src/ink/render-to-screen.ts", import.meta.url).href);
  const rendered = renderToScreen(element, 60);
  assert.equal(rendered.height, 3);
});
