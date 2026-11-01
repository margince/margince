// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Why a row is on the list and what it asks, where its details open.
//
// A row's reasons are a phrase line sized for scanning, and its consequence is
// drawn nowhere on it. A details view that leaves both out shows a title and a
// date. That says neither what the work is nor what to do about it.

import { type Fact, FactList } from "../../design-system/factlist";
import { Panel, PanelBody } from "../../design-system/panel";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { reasonText } from "../../lib/worklistcopy";
import { askKey } from "./worklist.ask";
import { useBriefAnswer, withFoldedBrief } from "./worklist.briefverbs";
import type { WorklistItem } from "./worklist.queries";
import { rowAnswer } from "./worklist.row.answer";

export function WorklistWhy({
  item,
  answer = false,
}: Readonly<{
  item: WorklistItem;
  // Draws the row's own verbs, for a drawer that covers the row it opened from.
  answer?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const facts = whyFacts(item, t, locale, viewerZone());
  return (
    <Panel title={t("worklist.why.title")}>
      <PanelBody>
        <div className="form-stack">
          <p>{t(askKey(item))}</p>
          {facts.length > 0 && <FactList facts={facts} />}
          {answer && <RowVerbs item={item} />}
        </div>
      </PanelBody>
    </Panel>
  );
}

// The same controls the row draws, so a drawer over the row answers it in place.
function RowVerbs({ item }: Readonly<{ item: WorklistItem }>) {
  const brief = useBriefAnswer(item);
  const placed = withFoldedBrief(rowAnswer(item, brief), item, brief);
  if (!placed.primary && !placed.equals && !placed.below) {
    return null;
  }
  return (
    <>
      <div className="form-actions">
        {placed.equals}
        {placed.primary}
      </div>
      {placed.below}
    </>
  );
}

// A fact the row does not carry is left out, not drawn blank. An empty value
// would claim the row has no reason.
function whyFacts(
  item: WorklistItem,
  t: Translator,
  locale: Locale,
  zone: string,
): Fact[] {
  const reasons = item.because
    .map((reason) => reasonText(reason, t, locale, zone))
    .filter((text): text is string => text !== null);
  const facts: Fact[] = [];
  if (reasons.length > 0) {
    facts.push({
      key: "reasons",
      term: t("worklist.why.reasons"),
      value: reasons.join(" · "),
    });
  }
  if (item.detail) {
    facts.push({
      key: "detail",
      term: t("worklist.why.detail"),
      value: item.detail,
    });
  }
  if (item.consequence !== "none") {
    facts.push({
      key: "consequence",
      term: t("worklist.why.consequence"),
      value: t(`worklist.consequence.${item.consequence}`),
    });
  }
  return facts;
}
