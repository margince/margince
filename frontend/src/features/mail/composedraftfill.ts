// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ProjectScope } from "../../design-system/projectpicker";
import { addressKey } from "../../lib/addresskey";
import type { CommunicationContext } from "../../lib/compose-context";
import { stripEveryKeyTag, withSubjectTag } from "../projects/projectrecord";
import { distinctAddressLines } from "./composeaddresslist";
import type {
  DraftProvenance,
  DraftReason,
  DraftResult,
} from "./composedraftcall";

/**
 * The reason a drafting instruction names. It fills an empty reason and
 * replaces or clears one an earlier instruction set; a reason the sender
 * picked stays.
 */
function takeDraftedReason(
  drafted: Pick<
    Extract<DraftResult, { available: true }>["draft"],
    "communication_context" | "evidence"
  >,
  form: Readonly<{
    fromInstruction: boolean;
    context: CommunicationContext | "";
    reasonFromDraft: boolean;
    setContext: (next: CommunicationContext | "") => void;
    setMeetingEvidence: (next: string | undefined) => void;
    setReasonFromDraft: (next: boolean) => void;
  }>,
): void {
  if (!form.fromInstruction || (form.context !== "" && !form.reasonFromDraft)) {
    return;
  }
  const reason = drafted.communication_context;
  if (!reason && !form.reasonFromDraft) return;
  form.setContext(reason ?? "");
  form.setMeetingEvidence(
    reason ? (drafted.evidence?.activity_id ?? undefined) : undefined,
  );
  form.setReasonFromDraft(Boolean(reason));
}

export function fillFromDraft(
  result: Extract<DraftResult, { available: true }>,
  form: Readonly<{
    subject: string;
    body: string;
    toEmpty: boolean;
    // The Cc and Bcc on screen: an address there stays the rep's, never a To.
    copies: readonly string[];
    setSubject: (next: string) => void;
    setBody: (next: string) => void;
    // The words as the model served them. A rewrite is offered only while
    // the body still equals them, because an edited body is the rep's work.
    setServedBody: (next: string) => void;
    // A rewrite, or a draft the route wrote from the body on screen, replaces
    // that body. Any other draft fills an empty field and leaves it alone.
    rewrite?: boolean;
    setTo: (next: string[]) => void;
    setDraftRef: (next: string | null) => void;
    setProvenance: (next: DraftProvenance) => void;
    setReasoning: (next: DraftReason[]) => void;
    setScope: (next: ProjectScope | undefined) => void;
    // What an instruction says the message is (rewrite verbs say nothing).
    fromInstruction: boolean;
    context: CommunicationContext | "";
    reasonFromDraft: boolean;
    setContext: (next: CommunicationContext | "") => void;
    setMeetingEvidence: (next: string | undefined) => void;
    setReasonFromDraft: (next: boolean) => void;
  }>,
): boolean {
  const drafted = result.draft;
  // A subject holding only the project tag has no words of the rep's yet, so
  // the drafted subject fills in behind the tag.
  const written = stripEveryKeyTag(form.subject).trim();
  if (!written) {
    form.setSubject(
      form.subject.trim()
        ? withSubjectTag(drafted.subject, form.subject.trim())
        : drafted.subject,
    );
  }
  const replacesBody = !form.body || form.rewrite === true;
  if (replacesBody) {
    // The reason follows the words on screen, so a held-back draft names none.
    takeDraftedReason(drafted, form);
    form.setBody(drafted.body);
    form.setServedBody(drafted.body);
    form.setDraftRef(drafted.draft_ref ?? null);
    form.setProvenance({
      // An absent flag reads as false: it may neither hide a disclosure nor
      // claim a model wrote untouched text.
      ai_generated: drafted.ai_generated ?? false,
      ai_disclosure: drafted.ai_disclosure,
      voice_profile_version: drafted.voice_profile_version,
      voice_degraded: drafted.voice_degraded ?? false,
      language: drafted.language,
      generated_by: drafted.generated_by,
    });
    form.setReasoning(result.reasoning ?? []);
    form.setScope(result.scope);
  }
  const to = form.toEmpty ? newRecipients(drafted.to ?? [], form.copies) : [];
  if (to.length > 0) {
    form.setTo(to);
  }
  return replacesBody;
}

function newRecipients(
  drafted: readonly string[],
  copies: readonly string[],
): string[] {
  const copied = new Set(copies.map(addressKey));
  const { to } = distinctAddressLines({ to: drafted, cc: [], bcc: [] });
  return to.filter((address) => !copied.has(addressKey(address)));
}
