// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { useT } from "../../i18n";
import { problemCodeOf } from "../../lib/problem";
import {
  type RewriteAbout,
  type RewriteVerb,
  rewriteWords,
} from "../../lib/rewrites";
import type { DraftAsk, DraftResult } from "./composedraftcall";

type Translate = ReturnType<typeof useT>;

// The record the composer writes from, which the rewrite path names: the
// message answered, else the record it was opened on.
function rewriteAbout(ask: DraftAsk): RewriteAbout {
  return ask.activityId
    ? { entity_type: "activity", entity_id: ask.activityId }
    : { entity_type: ask.entityType, entity_id: ask.entityId };
}

// A rewrite verb over the body on screen, answered as a draft. The fill and its
// clobber rule then read it like every other draft.
export async function rewriteAsDraft(
  ask: DraftAsk,
  verb: RewriteVerb,
  t: Translate,
): Promise<DraftResult> {
  const language = ask.language ?? ask.reader ?? "en";
  try {
    const rewritten = await rewriteWords(
      {
        envelope: "email",
        verb,
        body: ask.body,
        language,
        about: rewriteAbout(ask),
      },
      t,
    );
    return {
      available: true,
      draft: {
        subject: ask.subject ?? "",
        body: rewritten.body,
        to: [],
        ai_generated: true,
        ai_disclosure: rewritten.ai_disclosure,
        language: rewritten.language,
        voice_profile_version: rewritten.voice_profile_version,
        voice_degraded: rewritten.voice_degraded ?? false,
      },
      revised: true,
    };
  } catch (error) {
    if (problemCodeOf(error) === "ai_not_configured") {
      return { available: false, reason: "no_model" };
    }
    throw error;
  }
}
