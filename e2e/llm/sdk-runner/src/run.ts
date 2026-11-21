// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Drive one scenario prompt through the Claude Agent SDK, answering the
// questions the MCP server asks in the chat from the scenario's script.
//
// The SDK runs the same claude binary the CLI route runs, and its message
// stream is the CLI's stream-json, so every message is written as it comes and
// check.py reads the run as it reads a `claude -p` one. Each question and its
// answer are added as an `elicitation` line, which the checker passes over.
//
// Exit 0 when the run happened, whatever the model did, the turn cap included;
// exit 3 when it could not be run. Like the CLI route, no wall clock bounds a
// run beyond its turns. e2e/llm/drive.py starts this through clean.py's launch.

import { closeSync, mkdtempSync, openSync, readFileSync, rmSync, writeFileSync, writeSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { parseArgs } from "node:util";

import { query } from "@anthropic-ai/claude-agent-sdk";
import type { ElicitationRequest, ElicitationResult } from "@anthropic-ai/claude-agent-sdk";

import { record } from "./record.ts";
import { answer, claudeArgs, readReplies } from "./replies.ts";
import type { Reply } from "./replies.ts";

const HARNESS = 3;

const { values } = parseArgs({
  options: {
    model: { type: "string" },
    "mcp-url": { type: "string" },
    "token-env": { type: "string" },
    server: { type: "string" },
    "prompt-file": { type: "string" },
    "replies-file": { type: "string" },
    out: { type: "string" },
    cwd: { type: "string" },
    claude: { type: "string" },
    "max-turns": { type: "string", default: "20" },
    // The clean launch's flags, passed on to the binary as they are.
    "claude-arg": { type: "string", multiple: true, default: [] },
  },
  strict: true,
});

function required(name: keyof typeof values): string {
  const value = values[name];
  if (typeof value !== "string" || value === "") {
    throw new Error(`--${name} is required`);
  }
  return value;
}

class TranscriptFile {
  private readonly fd: number;

  constructor(path: string) {
    this.fd = openSync(path, "w", 0o600);
  }

  // Written line by line, so a run that dies mid-way leaves what it did.
  write(event: unknown): void {
    writeSync(this.fd, JSON.stringify(event) + "\n");
  }

  close(): void {
    closeSync(this.fd);
  }
}

function elicitationHandler(replies: readonly Reply[], out: TranscriptFile) {
  return async (request: ElicitationRequest): Promise<ElicitationResult> => {
    const reply = answer(replies, request.message);
    const result: ElicitationResult =
      reply.content === undefined ? { action: reply.action } : { action: reply.action, content: reply.content };
    out.write({
      type: "elicitation",
      server: request.serverName,
      mode: request.mode ?? "form",
      question: request.message,
      requested_schema: request.requestedSchema ?? null,
      reply: result,
      scripted_by: reply.scriptedBy,
    });
    return result;
  };
}

// mcpConfigFile writes the lane's one server where only this user can read it.
// The SDK would put mcpServers, and so the passport, on claude's command line.
function mcpConfigFile(dir: string, server: string, url: string, token: string): string {
  const path = join(dir, "mcp.json");
  const config = { mcpServers: { [server]: { type: "http", url, headers: { Authorization: `Bearer ${token}` } } } };
  writeFileSync(path, JSON.stringify(config), { mode: 0o600 });
  return path;
}

async function main(): Promise<number> {
  const server = required("server");
  const tokenEnv = required("token-env");
  const token = process.env[tokenEnv] ?? "";
  if (token === "") {
    throw new Error(`${tokenEnv} is not set`);
  }
  const prompt = readFileSync(required("prompt-file"), "utf8");
  const replies = values["replies-file"] ? readReplies(readFileSync(values["replies-file"], "utf8")) : [];
  const configDir = mkdtempSync(join(tmpdir(), "e2e-llm-sdk-mcp."));
  const out = new TranscriptFile(required("out"));
  try {
    const run = query({
      prompt,
      options: {
        cwd: required("cwd"),
        model: required("model"),
        pathToClaudeCodeExecutable: required("claude"),
        extraArgs: {
          ...claudeArgs(values["claude-arg"] ?? []),
          "mcp-config": mcpConfigFile(configDir, server, required("mcp-url"), token),
        },
        // The passport reaches claude in the config file alone.
        env: Object.fromEntries(Object.entries(process.env).filter(([name]) => name !== tokenEnv)),
        // The CLI route's own prompt, so the two routes differ only in who
        // answers the server's questions.
        systemPrompt: { type: "preset", preset: "claude_code" },
        strictMcpConfig: true,
        tools: [],
        allowedTools: [`mcp__${server}__*`],
        permissionMode: "dontAsk",
        maxTurns: Number(values["max-turns"]),
        onElicitation: elicitationHandler(replies, out),
      },
    });
    await record(run, (message) => out.write(message));
  } finally {
    out.close();
    rmSync(configDir, { recursive: true, force: true });
  }
  return 0;
}

main().then(
  (code) => process.exit(code),
  (crash: unknown) => {
    console.error(`the SDK runner crashed: ${crash instanceof Error ? crash.message : String(crash)}`);
    process.exit(HARNESS);
  },
);
