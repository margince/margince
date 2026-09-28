// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Deciding a Deal Scout suggestion where the Worklist shows it.
//
// The row carries the suggestion's id, its name and its company, which is
// enough to rank it and not enough to decide it: a rep opening a deal wants to
// see what the scout saw. So the card below the row reads the company's
// suggestions — the same read, under the same visibility rule, the company page
// makes — and draws this one with its evidence and its two answers.

import { useT } from "../i18n";
import { DealSuggestionCard } from "./dealsuggestion";
import { useDealSuggestions } from "./dealsuggestions.queries";
import type { WorklistItem } from "./worklist.queries";

export function SuggestionDecision({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const companyId = item.subject?.id;
  const query = useDealSuggestions(
    { company_id: companyId },
    Boolean(companyId),
  );
  const suggestion = query.data?.find((one) => one.id === item.id);
  if (!suggestion) {
    // Decided since the page was read, or no longer the reader's to see: the
    // row's own title still says what it was, and there is nothing to answer.
    return query.isPending ? null : (
      <p className="t-caption">{t("dealSuggestion.decided")}</p>
    );
  }
  return <DealSuggestionCard suggestion={suggestion} />;
}
