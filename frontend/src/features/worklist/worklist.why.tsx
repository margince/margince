// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Why a row is on the list and what it asks, where its details open.
//
// A row says one current reason. Its details list every concrete fact behind
// it, and leave out how the row ranks against the rest of the queue.

import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatActivityWhen } from "../../format/activitywhen";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useSourceOpener } from "../../lib/opensource";
import { useViewerZone } from "../../lib/viewerzone";
import { factReasons, partyName, reasonText } from "../../lib/worklistcopy";
import { askKey, saysOpen } from "./worklist.ask";
import { useBriefAnswer, withFoldedBrief } from "./worklist.briefverbs";
import type { WorklistItem } from "./worklist.queries";
import { codedDetail } from "./worklist.reader";
import { inlineDecision, rowAnswer } from "./worklist.row.answer";

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
          {saysOpen(item) && <p>{t(askKey(item))}</p>}
          <OurLastReply item={item} />
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
          {answer && <RowVerbs item={item} />}
        </div>
      </PanelBody>
    </Panel>
  );
}

// The reply a reader can open to judge for themselves a request Margince
// cannot check.
function OurLastReply({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const openerFor = useSourceOpener();
  const reply = item.request_check?.our_last_reply;
  if (!reply) return null;
  const label = t("worklist.ask.task_unchecked_reply", {
    date: formatActivityWhen(
      reply.occurred_at,
      reply.occurred_precision,
      locale,
      zone,
    ),
  });
  const open = openerFor({ id: reply.activity_id, kind: "email" });
  return (
    <p>
      {open ? (
        <Button variant="link" onClick={open}>
          {label}
        </Button>
      ) : (
        label
      )}
    </p>
  );
}

// The same controls the row draws, so a drawer over the row answers it in place.
// A decision is answered here, not behind a second drawer. The card takes the
// place of the Decide button.
function RowVerbs({ item }: Readonly<{ item: WorklistItem }>) {
  const brief = useBriefAnswer(item);
  const placed = withFoldedBrief(rowAnswer(item, brief), item, brief);
  const decision = inlineDecision(item);
  const primary = decision ? undefined : placed.primary;
  if (!decision && !primary && !placed.equals && !placed.below) {
    return null;
  }
  return (
    <>
      {decision}
      {(primary || placed.equals) && (
        <div className="form-actions">
          {placed.equals}
          {primary}
        </div>
      )}
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
