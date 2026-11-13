// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../api/client";
import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { unwrap } from "./problem";

export type RewriteAsk = components["schemas"]["RewriteRequest"];
export type RewriteVerb = RewriteAsk["verb"];
export type RewriteAbout = RewriteAsk["about"];
export type Rewritten = components["schemas"]["Rewrite"];

// What a rep asks the model to do to the words on screen, one press and one
// call each. The composer offers all; Add task and Log activity offer Polish.
// The server holds what each verb asks of the model; the client names it.
export const REWRITES = [
  { key: "polish", label: "compose.rewritePolish" },
  { key: "shorter", label: "compose.rewriteShorter" },
  { key: "warmer", label: "compose.rewriteWarmer" },
  { key: "formal", label: "compose.rewriteFormal" },
  { key: "deadline", label: "compose.rewriteDeadline" },
] as const satisfies readonly { key: RewriteVerb; label: MessageKey }[];

/**
 * The most characters one rewrite takes, the server's own limit. A declared
 * mirror of rewrite.MaxSourceRunes; backend/gates/rewritelimit_test.go holds it.
 */
export const REWRITE_MAX_CHARACTERS = 12_288;

/** The verb Add task and Log activity offer as Polish wording. */
export const POLISH = REWRITES[0];

/**
 * The one rewrite call, behind the composer's verbs and Polish wording alike.
 * A refusal throws as a problem the caller reads by its code.
 */
export async function rewriteWords(
  ask: RewriteAsk,
  t?: (key: MessageKey) => string,
): Promise<Rewritten> {
  return unwrap(await api.POST("/rewrites", { body: ask }), t);
}
