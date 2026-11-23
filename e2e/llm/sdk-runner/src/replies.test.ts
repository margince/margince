// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

import { answer, claudeArgs, readReplies } from "./replies.ts";

const BULK = "Give a new owner to 12 contacts? You can undo it afterwards.";

test("the first entry the question holds answers it, with its content", () => {
  const replies = readReplies(
    JSON.stringify([
      { match: "give a new owner", action: "accept", content: { confirm: true } },
      { match: "owner", action: "decline" },
    ]),
  );
  assert.deepEqual(answer(replies, BULK), {
    action: "accept",
    content: { confirm: true },
    scriptedBy: "give a new owner",
  });
});

test("a decline carries no content", () => {
  const replies = readReplies(JSON.stringify([{ match: "new owner", action: "decline" }]));
  assert.deepEqual(answer(replies, BULK), { action: "decline", scriptedBy: "new owner" });
});

test("a question no entry names is cancelled and marked unscripted", () => {
  const replies = readReplies(JSON.stringify([{ match: "Undo the bulk change", action: "accept" }]));
  assert.deepEqual(answer(replies, BULK), { action: "cancel", scriptedBy: null });
});

test("an accept with no content answers an empty form", () => {
  const replies = readReplies(JSON.stringify([{ match: "new owner", action: "accept" }]));
  assert.deepEqual(answer(replies, BULK).content, {});
});

test("a script the runner cannot apply is refused", () => {
  assert.throws(() => readReplies("{}"), /JSON list/);
  assert.throws(() => readReplies(JSON.stringify([{ match: "", action: "accept" }])), /no match text/);
  assert.throws(() => readReplies(JSON.stringify([{ match: "x", action: "yes" }])), /not accept, decline or cancel/);
  assert.throws(
    () => readReplies(JSON.stringify([{ match: "x", action: "accept", content: { pick: { id: 1 } } }])),
    /no form field holds/,
  );
});

test("the clean launch's flags become extraArgs, an empty value kept", () => {
  assert.deepEqual(claudeArgs(["--setting-sources", "", "--disable-slash-commands"]), {
    "setting-sources": "",
    "disable-slash-commands": null,
  });
  assert.throws(() => claudeArgs(["stray"]), /no flag before it/);
});

// The answers check.py's parser writes, and the ones it refuses, read the same way here.
const SHARED = JSON.parse(readFileSync(new URL("../testdata/replies.json", import.meta.url), "utf8"));

test("every reply check.py writes is read back unchanged", () => {
  assert.deepEqual(readReplies(JSON.stringify(SHARED.replies)), SHARED.replies);
});

test("every reply check.py refuses to write is refused here too", () => {
  for (const reply of SHARED.refused_replies) {
    assert.throws(() => readReplies(JSON.stringify([reply])), Error, JSON.stringify(reply));
  }
});
