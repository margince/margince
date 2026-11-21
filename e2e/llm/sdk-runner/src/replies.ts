// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The user's scripted answers to the questions an MCP server asks in the chat.
//
// check.py parses a scenario's `elicitation:` entries into the JSON this reads.
// This side refuses what that parser never writes, and testdata/replies.json
// holds both sides to the same answers.

export type Action = "accept" | "decline" | "cancel";

// What a form field can hold, as the MCP specification's ElicitResult has it.
export type FormValue = string | number | boolean | string[];
export type FormContent = Record<string, FormValue>;

export type Reply = {
  match: string;
  action: Action;
  content?: FormContent;
};

export type Answer = {
  action: Action;
  content?: FormContent;
  // The entry that answered, or null when no entry matched the question.
  scriptedBy: string | null;
};

function isAction(value: unknown): value is Action {
  return value === "accept" || value === "decline" || value === "cancel";
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isFormValue(value: unknown): value is FormValue {
  if (Array.isArray(value)) {
    return value.every((item) => typeof item === "string");
  }
  return typeof value === "string" || typeof value === "number" || typeof value === "boolean";
}

function formContent(content: Record<string, unknown>, index: number): FormContent {
  const kept: FormContent = {};
  for (const [key, value] of Object.entries(content)) {
    if (!isFormValue(value)) {
      throw new Error(`reply ${index} gives ${key} a value no form field holds`);
    }
    kept[key] = value;
  }
  return kept;
}

// readReplies refuses a file whose entries this runner cannot apply, so a
// malformed script stops the run instead of answering a question wrongly.
export function readReplies(text: string): Reply[] {
  const parsed: unknown = JSON.parse(text);
  if (!Array.isArray(parsed)) {
    throw new Error("the replies file must hold a JSON list");
  }
  return parsed.map((entry: unknown, index): Reply => {
    if (!isObject(entry)) {
      throw new Error(`reply ${index} is not an object`);
    }
    const { match, action, content } = entry;
    if (typeof match !== "string" || match === "") {
      throw new Error(`reply ${index} has no match text`);
    }
    if (!isAction(action)) {
      throw new Error(`reply ${index} has action ${String(action)}, not accept, decline or cancel`);
    }
    if (content === undefined) {
      return { match, action };
    }
    if (action !== "accept") {
      throw new Error(`reply ${index} gives content to a ${action}, which sends none`);
    }
    if (!isObject(content)) {
      throw new Error(`reply ${index} has content that is not an object`);
    }
    return { match, action, content: formContent(content, index) };
  });
}

// answer picks the first entry whose text the question's message holds, case
// aside. A question no entry names is cancelled, as a host with nobody at the
// keyboard would; the transcript records that it was not scripted.
export function answer(replies: readonly Reply[], message: string): Answer {
  const asked = message.toLowerCase();
  const hit = replies.find((reply) => asked.includes(reply.match.toLowerCase()));
  if (!hit) {
    return { action: "cancel", scriptedBy: null };
  }
  if (hit.action === "accept") {
    return { action: "accept", content: hit.content ?? {}, scriptedBy: hit.match };
  }
  return { action: hit.action, scriptedBy: hit.match };
}

// claudeArgs turns the clean launch's `--flag value` and bare `--flag` tokens
// into the SDK's extraArgs, where a flag with no value is null. An empty string
// is a value: `--setting-sources ""` loads no settings.
export function claudeArgs(tokens: readonly string[]): Record<string, string | null> {
  const extra: Record<string, string | null> = {};
  for (let i = 0; i < tokens.length; i++) {
    const flag = tokens[i] ?? "";
    if (!flag.startsWith("--")) {
      throw new Error(`--claude-arg ${flag} is a value with no flag before it`);
    }
    const next = tokens[i + 1];
    if (next !== undefined && !next.startsWith("--")) {
      extra[flag.slice(2)] = next;
      i++;
    } else {
      extra[flag.slice(2)] = null;
    }
  }
  return extra;
}
