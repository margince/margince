// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Deciding a Deal Scout suggestion where the Worklist shows it.
//
// The row carries the suggestion's id, its name and its company, which is
// enough to rank it and not enough to decide it: a rep opening a deal wants to
// see what the scout saw. So the card below the row reads the company's
// suggestions — the same read, under the same visibility rule, the company page
// makes — and draws this one with its evidence and its two answers.

import type { ReactNode } from "react";
import { useT } from "../i18n";
import { DealSuggestionCard } from "./dealsuggestion";
import { useDealSuggestions } from "./dealsuggestions.queries";
import { PairDecision } from "./worklist.pair";
import type { WorklistItem } from "./worklist.queries";
import { TagSuggestionDecision } from "./worklist.tagsuggestion";

export function SuggestionDecision({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const companyId = item.subject?.id;
  const query = useDealSuggestions(
    { company_id: companyId },
    Boolean(companyId),
  );
  const suggestion = query.data?.find((one) => one.id === item.id);
  if (suggestion) {
    return <DealSuggestionCard suggestion={suggestion} />;
  }
  if (query.isPending) {
    return null;
  }
  // A read that failed says nothing about the suggestion, so it must not be
  // told as a decision somebody made.
  if (query.isError) {
    return <p className="t-caption">{t("dealSuggestion.unavailable")}</p>;
  }
  // Read, and not among them: decided since the page was read, or no longer
  // the reader's to see. The row still says what it was.
  return <p className="t-caption">{t("dealSuggestion.decided")}</p>;
}

// The answers too large for the row's own line, drawn UNDER it.
export function answerBelow(
  item: WorklistItem,
): { below: ReactNode } | undefined {
  if (item.source === "dedupe_candidate" && item.pair) {
    // UNDER the row, not in it. Each of its two verbs names the record it
    // would keep and stands in the list entry that describes that record —
    // lifted out into a row of verbs, "Keep Acme GmbH" and "Keep Acme GmbH"
    // would be two identical buttons over an irreversible merge.
    return { below: <PairDecision item={item} /> };
  }
  // A suggestion's evidence is what a rep reads before opening a deal, and it
  // does not fit on one line either. Never on a batch, which names no single
  // suggestion.
  if (item.source === "deal_suggestion" && !item.batch) {
    return { below: <SuggestionDecision item={item} /> };
  }
  // The suggested word and the mail that raised it, with Accept and Dismiss.
  if (item.source === "tag_suggestion" && !item.batch) {
    return { below: <TagSuggestionDecision item={item} /> };
  }
  return undefined;
}
