// How a choice card acts on the user's click: it asks the HOST to run one of the
// tools the view declared in actions.json, and to say so in the chat when the
// host will not.
//
// A view that does not import this file carries none of it, which is why the
// admission check can refuse the call method outright in every document that
// does not declare an action. The host, not the view, holds the credential: the
// call arrives at the server as the connected assistant, through the same
// passport, scope and approval checks as a call the model made.

import { nextRequestID, onResponse, sendToHost } from "./bridge";
import { asRecord, asText, asWarnings, type Warning } from "./types";

/** How long a click waits for the host before the card says it did not land. */
export const CALL_TIMEOUT_MS = 30_000;

/** What a click came to. `reason` is the server's own refusal text where it
 *  gave one, shown as text and never parsed. */
export type ActionOutcome =
  | { ok: true; data: unknown; warnings: Warning[] }
  | { ok: false; reason: string };

type Pending = {
  resolve: (outcome: ActionOutcome) => void;
  timer: ReturnType<typeof setTimeout>;
};

const pending = new Map<number, Pending>();
let declared: ReadonlySet<string> = new Set();
let hostProxiesTools = false;

/** declareActions names the tools this view may ask the host to run. The list
 *  is actions.json's entry for the view, so the build, the server's admission
 *  check and this guard read one source. */
export function declareActions(names: readonly string[]): void {
  declared = new Set(names);
}

/** canCallTools reports whether the host announced it will proxy tool calls. A
 *  card whose host will not shows "Ask the assistant" instead of a button that
 *  would never answer. */
export function canCallTools(): boolean {
  return hostProxiesTools;
}

/** callServerTool asks the host to run a declared tool and resolves with what
 *  it came to. It never rejects: a refusal, a host error and a silence are all
 *  outcomes the card draws. */
export function callServerTool(
  name: string,
  args: Record<string, unknown>,
): Promise<ActionOutcome> {
  if (!declared.has(name)) {
    return Promise.resolve({
      ok: false,
      reason: `This view may not run "${name}".`,
    });
  }
  if (!hostProxiesTools) {
    return Promise.resolve({
      ok: false,
      reason: "This host does not run tools from a view.",
    });
  }
  const id = nextRequestID();
  return new Promise((resolve) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      resolve({ ok: false, reason: "The host did not answer in time." });
    }, CALL_TIMEOUT_MS);
    pending.set(id, { resolve, timer });
    sendToHost({
      id,
      method: "tools/call",
      params: { name, arguments: args },
    });
  });
}

/** askAssistant puts the user's choice in the chat as a message, for a host
 *  that will not proxy a call. The model then runs the same governed tool. */
export function askAssistant(text: string): void {
  sendToHost({
    id: nextRequestID(),
    method: "ui/message",
    params: { role: "user", content: { type: "text", text } },
  });
}

function outcomeOf(message: Record<string, unknown>): ActionOutcome {
  if ("error" in message) {
    const reason = asText(asRecord(message.error).message);
    return {
      ok: false,
      reason: reason === "" ? "The host refused the call." : reason,
    };
  }
  const result = asRecord(message.result);
  if (result.isError === true) {
    const first = asRecord(
      Array.isArray(result.content) ? result.content[0] : null,
    );
    const reason = asText(first.text);
    return {
      ok: false,
      reason: reason === "" ? "The tool refused the call." : reason,
    };
  }
  const envelope = asRecord(result.structuredContent);
  return {
    ok: true,
    data: envelope.data ?? null,
    warnings: asWarnings(envelope.warnings),
  };
}

function settle(message: Record<string, unknown>): void {
  const id = message.id;
  if (typeof id !== "number") return;
  const waiting = pending.get(id);
  if (waiting === undefined) return;
  clearTimeout(waiting.timer);
  pending.delete(id);
  waiting.resolve(outcomeOf(message));
}

function learnCapabilities(hostCapabilities: unknown): void {
  hostProxiesTools = asRecord(hostCapabilities).serverTools !== undefined;
}

onResponse(settle, learnCapabilities);
