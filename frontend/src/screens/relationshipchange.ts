// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What HAPPENED to a relationship, as one sentence.
//
// Two surfaces say it — the contact 360's rail and the contact's Network tab —
// and a second spelling would give one derived change two sets of words. A
// reader meeting "Replied after a silence" on one screen and "They replied
// after 41 quiet days" on the other would reasonably ask which one the product
// means.

import type { components } from "../api/schema";
import type { useT } from "../i18n";

type ContactRelationshipChange =
  components["schemas"]["ContactRelationshipChange"];

/**
 * changeSentence writes one derived change as a sentence. A band move names
 * BOTH bands: "the relationship moved" without saying from what to what is a
 * claim the reader has to take on trust.
 */
export function changeSentence(
  change: ContactRelationshipChange,
  t: ReturnType<typeof useT>,
): string {
  const days = String(change.days ?? 0);
  if (change.kind === "replied_after_gap") {
    return t("contact.change.repliedAfterGap", { days });
  }
  if (change.kind === "went_quiet") {
    return t("contact.change.wentQuiet", { days });
  }
  return t(`contact.change.${change.kind}`, {
    from: t(`contact.band.${change.from_bucket ?? "none"}`),
    to: t(`contact.band.${change.to_bucket ?? "none"}`),
  });
}
