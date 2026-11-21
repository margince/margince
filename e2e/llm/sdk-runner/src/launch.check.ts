// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How the runner starts claude, checked against the installed SDK with a claude
// that only records what it was given. It needs the SDK installed, so the lane
// runs it before a paid run (npm run selfcheck) and the free suite does not.

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { chmodSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";

const PASSPORT = "mgp_planted-passport-value";

// The stand-in records its argv, its environment and the MCP config it was
// pointed at, then exits as a claude that failed to start would.
const FAKE_CLAUDE = `#!/usr/bin/env node
const fs = require("node:fs");
const argv = process.argv.slice(2);
const at = argv.indexOf("--mcp-config");
const config = at >= 0 ? fs.readFileSync(argv[at + 1], "utf8") : "";
const mode = at >= 0 ? (fs.statSync(argv[at + 1]).mode & 0o777) : 0;
fs.writeFileSync(process.env.FAKE_CLAUDE_SEEN, JSON.stringify({ argv, env: process.env, config, mode }));
process.exit(1);
`;

test("the passport reaches claude in a private config file, never its argv or environment", () => {
  const dir = mkdtempSync(join(tmpdir(), "e2e-llm-sdk-check."));
  const claude = join(dir, "claude.cjs");
  writeFileSync(claude, FAKE_CLAUDE);
  chmodSync(claude, 0o755);
  writeFileSync(join(dir, "prompt.txt"), "List the pipelines.");
  const seen = join(dir, "seen.json");
  const runner = new URL("./run.ts", import.meta.url).pathname;
  const done = spawnSync(
    "node",
    [
      runner, "--model", "claude-sonnet-5-5", "--mcp-url", "http://127.0.0.1:9/mcp",
      "--token-env", "MARGINCE_E2E_TOKEN", "--server", "margince_e2e_llm",
      "--prompt-file", join(dir, "prompt.txt"), "--out", join(dir, "run.jsonl"), "--cwd", dir,
      "--claude", claude, "--claude-arg=--setting-sources", "--claude-arg=",
    ],
    { env: { ...process.env, MARGINCE_E2E_TOKEN: PASSPORT, FAKE_CLAUDE_SEEN: seen }, encoding: "utf8" },
  );
  assert.equal(done.status, 3, "a claude that exits before any result is a harness stop");
  const recorded = JSON.parse(readFileSync(seen, "utf8"));
  assert.ok(!JSON.stringify(recorded.argv).includes(PASSPORT), "the passport is on claude's command line");
  assert.ok(!JSON.stringify(recorded.env).includes(PASSPORT), "the passport is in claude's environment");
  assert.ok(recorded.config.includes(`Bearer ${PASSPORT}`), "the config file does not carry the passport");
  assert.equal(recorded.mode, 0o600);
  assert.ok(recorded.argv.includes("--strict-mcp-config"));
});
