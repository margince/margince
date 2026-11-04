// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Why a row is on the list and what it asks, where its details open.
//
// A row's reasons are a phrase line sized for scanning, and its consequence is
// drawn nowhere on it. A details view that leaves both out shows a title and a
// date. That says neither what the work is nor what to do about it.

import { Panel, PanelBody } from "../../design-system/panel";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { reasonText } from "../../lib/worklistcopy";
import { askKey } from "./worklist.ask";
import { useBriefAnswer, withFoldedBrief } from "./worklist.briefverbs";
import type { WorklistItem } from "./worklist.queries";
import { rowAnswer } from "./worklist.row.answer";

import "./worklist.why.css";

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
  const reasons = whyReasons(item, t, locale, viewerZone());
  return (
    <Panel title={t("worklist.why.title")} tone="accent">
      <PanelBody>
        <div className="form-stack">
          <p>{t(askKey(item))}</p>
          {reasons.length > 0 && (
            <ol
              className="worklist-why-reasons"
              aria-label={t("worklist.why.reasons")}
            >
              {reasons.map((reason, index) => (
                <li key={`${index}:${reason}`}>{reason}</li>
              ))}
            </ol>
          )}
          {item.detail && (
            <figure className="worklist-why-quote">
              <figcaption className="t-caption">
                {t("worklist.why.detail")}
              </figcaption>
              <blockquote>{item.detail}</blockquote>
            </figure>
          )}
          {item.consequence !== "none" && (
            <p className="worklist-why-if">
              <span className="t-caption">{t("worklist.why.consequence")}</span>{" "}
              {t(`worklist.consequence.${item.consequence}`)}
            </p>
          )}
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

// A reason with no words in this language is left out, not drawn blank.
function whyReasons(
  item: WorklistItem,
  t: Translator,
  locale: Locale,
  zone: string,
): string[] {
  return item.because
    .map((reason) => reasonText(reason, t, locale, zone))
    .filter((text): text is string => text !== null);
}
