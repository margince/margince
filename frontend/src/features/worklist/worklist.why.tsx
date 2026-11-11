// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Why a row is on the list and what it asks, where its details open.
//
// A row says one current reason. Its details list every concrete fact behind
// it, and leave out how the row ranks against the rest of the queue.

import { Panel, PanelBody } from "../../design-system/panel";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";
import { factReasons, partyName, reasonText } from "../../lib/worklistcopy";
import { WebsiteInquiryFacts } from "./websiteinquiryfacts";
import { askKey } from "./worklist.ask";
import { useBriefAnswer, withFoldedBrief } from "./worklist.briefverbs";
import type { WorklistItem } from "./worklist.queries";
import { codedDetail } from "./worklist.reader";
import { rowAnswer } from "./worklist.row.answer";

import "./worklist.why.css";

const QUOTED_SOURCE = "conversation_claim";

export function WorklistWhy({
  item,
  answer = false,
}: Readonly<{
  item: WorklistItem;
  // Draws the row's own verbs, for a drawer that covers the row it opened from.
  answer?: boolean;
}>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const reasons = whyReasons(item, t, locale, readerZone);
  const detail = codedDetail(item, t);
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
              {reasons.map((reason) => (
                <li key={reason}>{reason}</li>
              ))}
            </ol>
          )}
          {detail &&
            // Only a promise's detail is verbatim source evidence, quoted from a
            // message; every other detail is the product's explanation.
            (item.source === QUOTED_SOURCE ? (
              <figure className="worklist-why-quote">
                <figcaption className="t-caption">
                  {t("worklist.why.detail")}
                </figcaption>
                <blockquote>{detail}</blockquote>
              </figure>
            ) : (
              <p>{detail}</p>
            ))}
          {item.website_inquiry && (
            <WebsiteInquiryFacts facts={item.website_inquiry} />
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
  // A reason said twice is read once, and the text is then its own key.
  const said = factReasons(item)
    .map((reason) => reasonText(reason, t, locale, zone, partyName(item)))
    .filter((text): text is string => text !== null);
  return [...new Set(said)];
}
