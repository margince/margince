// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import assert from "node:assert/strict";
import { test } from "node:test";

import { record } from "./record.ts";

const EXITED = new Error("Claude Code process exited with code 1");

async function* stream(messages: unknown[], thenThrow: boolean): AsyncGenerator<unknown> {
  for (const message of messages) {
    yield message;
  }
  if (thenThrow) {
    throw EXITED;
  }
}

test("a nonzero exit after the result is the run's own end, and the run is kept", async () => {
  const written: unknown[] = [];
  const capped = { type: "result", subtype: "error_max_turns", is_error: true };
  await record(stream([{ type: "assistant" }, capped], true), (message) => written.push(message));
  assert.deepEqual(written, [{ type: "assistant" }, capped]);
});

test("a nonzero exit before any result is a run that never finished", async () => {
  const written: unknown[] = [];
  await assert.rejects(record(stream([{ type: "assistant" }], true), (message) => written.push(message)), EXITED);
  assert.deepEqual(written, [{ type: "assistant" }]);
});

test("a clean stream is written whole", async () => {
  const written: unknown[] = [];
  await record(stream([{ type: "system" }, { type: "result" }], false), (message) => written.push(message));
  assert.equal(written.length, 2);
});
