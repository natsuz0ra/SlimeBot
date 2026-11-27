import assert from "node:assert/strict";
import test from "node:test";
import {
  formatCompactNumber,
  formatUsageRowLabel,
  getOutputBudgetStatus,
  resolveUsageTimeRange,
  summarizeUsageRows,
} from "./usage.js";

test("getOutputBudgetStatus computes normal, warning, and danger tones", () => {
  assert.equal(getOutputBudgetStatus(null), null);
  assert.equal(getOutputBudgetStatus({ outputBudgetTokens: 0, outputBudgetUsed: 500 }), null);

  const normal = getOutputBudgetStatus({ outputBudgetTokens: 10_000, outputBudgetUsed: 4_000 });
  assert.deepEqual(normal, {
    used: 4_000,
    budget: 10_000,
    remaining: 6_000,
    percent: 40,
    tone: "normal",
    exceeded: false,
  });

  const warning = getOutputBudgetStatus({ outputBudgetTokens: 10_000, outputBudgetUsed: 8_200 });
  assert.equal(warning?.tone, "warning");
  assert.equal(warning?.exceeded, false);

  const danger = getOutputBudgetStatus({ outputBudgetTokens: 10_000, outputBudgetUsed: 12_500 });
  assert.equal(danger?.tone, "danger");
  assert.equal(danger?.exceeded, true);
  assert.equal(danger?.remaining, 0);
});

test("resolveUsageTimeRange returns empty object for all and ISO since for windows", () => {
  const now = new Date("2026-11-27T12:00:00.000Z");
  assert.deepEqual(resolveUsageTimeRange("all", now), {});
  const today = resolveUsageTimeRange("today", now);
  const week = resolveUsageTimeRange("7d", now);
  const month = resolveUsageTimeRange("30d", now);
  assert.ok(today.since && week.since && month.since);
  assert.ok(Date.parse(month.since) < Date.parse(week.since));
  assert.ok(Date.parse(week.since) < Date.parse(today.since));
});

test("formatCompactNumber formats small, thousand, and million values", () => {
  assert.equal(formatCompactNumber(0), "0");
  assert.equal(formatCompactNumber(842), "842");
  assert.equal(formatCompactNumber(12_460), "12.5k");
  assert.equal(formatCompactNumber(2_340_000), "2.34m");
});

test("summarizeUsageRows aggregates totals and formatUsageRowLabel handles deleted sessions", () => {
  const rows = [
    {
      groupKey: "s1",
      rootSessionId: "s1",
      sessionName: "Fix Bug",
      sessionExists: true,
      requestCount: 3,
      inputTokens: 1000,
      outputTokens: 400,
      cacheReadInputTokens: 200,
    },
    {
      groupKey: "s2",
      rootSessionId: "s2",
      sessionName: "Old Chat",
      sessionExists: false,
      requestCount: 2,
      inputTokens: 500,
      outputTokens: 100,
      cacheReadInputTokens: 50,
    },
  ];

  assert.deepEqual(summarizeUsageRows(rows), {
    requests: 5,
    input: 1500,
    output: 500,
    cacheRead: 250,
  });
  assert.equal(formatUsageRowLabel("session", rows[0]), "Fix Bug");
  assert.equal(formatUsageRowLabel("session", rows[1]), "Old Chat (deleted)");
});
