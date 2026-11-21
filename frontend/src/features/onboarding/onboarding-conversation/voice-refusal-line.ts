import { formatBytes } from "../../../format/format";
import type { Locale, Translator } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { problemMessage } from "../../../lib/problem";
import { problemCode } from "../../../lib/problembody";
import { PROBLEM_CODE_COPY } from "../../../lib/problemcodecopy";
import type { IntakeOutcome } from "../../../lib/voice-intake-core";
import {
  type RefusalReason,
  VOICE_SOURCE_MAX_BYTES,
} from "../../../lib/voice-refusal";
import type { ConversationEvent } from "./conversation-machine";

const refusalKeys: Record<RefusalReason, MessageKey> = {
  unattributed: "ob.conv.voice.refusalUnattributed",
  speaker: "ob.conv.voice.refusalSpeaker",
  unsupported: "ob.conv.voice.refusalUnsupported",
  tooLarge: "ob.conv.voice.refusalTooLarge",
};

// Why a file went nowhere, in the room's own words. `type` is decided by name
// before anything is read; the other two are what reading it found.
export const skipKeys: Record<
  Extract<IntakeOutcome, { kind: "skipped" }>["reason"],
  MessageKey
> = {
  type: "ob.conv.voice.fileSkipped",
  empty: "ob.conv.voice.fileEmpty",
  unreadable: "ob.conv.voice.fileUnreadable",
};

// The refusal category the core read off the 422 picks the line. A refusal it
// did not recognize falls back to the server's own safe detail.
export function refusalEntry(
  outcome: Extract<IntakeOutcome, { kind: "refused" }>,
  locale: Locale,
  t: Translator,
): Extract<ConversationEvent, { type: "NARRATION" }>["entry"] {
  const { ref, reason, problem } = outcome;
  if (reason !== null) {
    return {
      kind: "narration",
      id: `refuse:${ref}`,
      i18nKey: refusalKeys[reason],
      params: {
        name: outcome.label,
        limit: formatBytes(VOICE_SOURCE_MAX_BYTES, locale),
      },
    };
  }
  // A code the catalog words (a role refused the grant) is said in the
  // reader's language, never as the server's English detail.
  const codeKey = PROBLEM_CODE_COPY.get(problemCode(problem) ?? "");
  return {
    kind: "narration",
    id: `refuse:${ref}`,
    i18nKey: "ob.conv.voice.ingestFailed",
    params: {
      name: outcome.label,
      detail: codeKey === undefined ? problemMessage(problem) : t(codeKey),
    },
  };
}
