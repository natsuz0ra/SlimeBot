/**
 * SlimeBot CLI — Ink-based terminal UI entry point.
 *
 * Usage:
 *   slimebot cli          (started automatically by the Go backend)
 *   node cli/dist/index.js --api-url http://... --cli-token xxx
 *
 * Environment variables:
 *   SLIMEBOT_API_URL   — Go headless server URL
 *   SLIMEBOT_CLI_TOKEN — CLI auth token
 */

import React from "react";
import { render } from "ink";
import { ArgumentParser } from "argparse";
import { App } from "./app.js";

function parseArgs(): { apiUrl: string; cliToken: string } {
  const parser = new ArgumentParser({
    description: "SlimeBot CLI — AI chat in your terminal",
  });
  parser.add_argument("--api-url", {
    help: "Go headless server URL (default: from SLIMEBOT_API_URL env)",
    default: process.env.SLIMEBOT_API_URL || "http://127.0.0.1:8080",
  });
  parser.add_argument("--cli-token", {
    help: "CLI auth token (default: from SLIMEBOT_CLI_TOKEN env)",
    default: process.env.SLIMEBOT_CLI_TOKEN || "",
  });
  const args = parser.parse_args();
  return {
    apiUrl: args.api_url as string,
    cliToken: args.cli_token as string,
  };
}

function main(): void {
  const { apiUrl, cliToken } = parseArgs();

  if (!cliToken) {
    process.stderr.write(
      "Error: CLI token is required. Set SLIMEBOT_CLI_TOKEN or use --cli-token.\n",
    );
    process.exit(1);
  }

  const version = "1.18.1";

  render(
    <App
      apiURL={apiUrl}
      cliToken={cliToken}
      version={version}
    />,
  );
}

main();
