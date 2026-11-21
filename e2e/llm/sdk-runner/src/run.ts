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
// Exit 0 when the run happened, whatever the model did; exit 3 when it could
// not be run. e2e/llm/drive.py starts this through clean.py's launch.

import { closeSync, openSync, readFileSync, writeSync } from "node:fs";
import { parseArgs } from "node:util";

import { query } from "@anthropic-ai/claude-agent-sdk";
import type { ElicitationRequest, ElicitationResult } from "@anthropic-ai/claude-agent-sdk";

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
    "timeout-s": { type: "string", default: "900" },
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

async function main(): Promise<number> {
  const server = required("server");
  const token = process.env[required("token-env")] ?? "";
  if (token === "") {
    throw new Error(`${values["token-env"]} is not set`);
  }
  const prompt = readFileSync(required("prompt-file"), "utf8");
  const replies = values["replies-file"] ? readReplies(readFileSync(values["replies-file"], "utf8")) : [];
  const out = new TranscriptFile(required("out"));
  const abort = new AbortController();
  const timer = setTimeout(() => abort.abort(), Number(values["timeout-s"]) * 1000);
  try {
    const run = query({
      prompt,
      options: {
        abortController: abort,
        cwd: required("cwd"),
        model: required("model"),
        pathToClaudeCodeExecutable: required("claude"),
        extraArgs: claudeArgs(values["claude-arg"] ?? []),
        // The CLI route's own prompt, so the two routes differ only in who
        // answers the server's questions.
        systemPrompt: { type: "preset", preset: "claude_code" },
        mcpServers: {
          [server]: { type: "http", url: required("mcp-url"), headers: { Authorization: `Bearer ${token}` } },
        },
        strictMcpConfig: true,
        tools: [],
        allowedTools: [`mcp__${server}__*`],
        permissionMode: "dontAsk",
        maxTurns: Number(values["max-turns"]),
        onElicitation: elicitationHandler(replies, out),
      },
    });
    for await (const message of run) {
      out.write(message);
    }
  } finally {
    clearTimeout(timer);
    out.close();
  }
  if (abort.signal.aborted) {
    console.error(`the run did not finish within ${values["timeout-s"]} seconds`);
    return HARNESS;
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
