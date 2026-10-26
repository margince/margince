// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Why a row is on the list, where its details open.
//
// A row's reasons are a phrase line sized for scanning, and its consequence is
// drawn nowhere on it. A details view that leaves both out shows a title and a
// date. That says neither what the work is nor why it matters now.

import { type Fact, FactList } from "../../design-system/factlist";
import { Panel, PanelBody } from "../../design-system/panel";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { reasonText } from "./worklist.copy";
import type { WorklistItem } from "./worklist.queries";
import { known } from "./worklist.reasoncensus";

export function WorklistWhy({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  if (!hasWhy(item)) {
    return null;
  }
  return (
    <Panel title={t("worklist.why.title")}>
      <PanelBody>
        <FactList facts={whyFacts(item, t, locale, viewerZone())} />
      </PanelBody>
    </Panel>
  );
}

/** Whether the panel has a fact to draw: the three `whyFacts` can add. A
 *  reason this build has no sentence for draws nothing, as in `reasonText`. */
export function hasWhy(item: WorklistItem): boolean {
  return (
    item.because.some((reason) => known(reason.kind)) ||
    Boolean(item.detail) ||
    item.consequence !== "none"
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
