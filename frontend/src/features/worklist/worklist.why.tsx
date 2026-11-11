// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Why a row is on the list and what it asks, where its details open.
//
// A row says one current reason. Its details say what happened, every concrete
// fact behind it, what waiting costs and what to do. They leave out how the row
// ranks against the rest of the queue.

import type { ReactNode } from "react";
import { Button } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatActivityWhen } from "../../format/activitywhen";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useSourceOpener } from "../../lib/opensource";
import { useViewerZone } from "../../lib/viewerzone";
import { saysOpen } from "../../lib/worklistask";
import { consequenceLine } from "../../lib/worklistconsequence";
import {
  factReasons,
  partyName,
  reasonText,
  saidByTheWhenLine,
} from "../../lib/worklistcopy";
import {
  happenedLines,
  todoLine,
  useHappenedClock,
} from "../../lib/worklisthappened";
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
  const t = useT();
  const clock = useHappenedClock();
  const { lines: happened, told } = happenedLines(item, t, clock);
  const reasons = whyReasons(
    item,
    told && Boolean(item.due_at) && DATED_SOURCES.has(item.source),
    t,
    clock.locale,
    clock.zone,
  );
  const consequence = consequenceLine(item, t);
  const detail = codedDetail(item, t);
  // A row asking whether our later mail kept its promise has no ask to say.
  const open = saysOpen(item);
  return (
    <Panel title={t("worklist.why.title")} tone="accent">
      <PanelBody>
        <div className="worklist-why">
          {(happened.length > 0 || detail) && (
            <WhySection heading={t("worklist.why.happened")}>
              {happened.length > 0 && (
                <ul className="worklist-why-happened">
                  {happened.map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
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
            </WhySection>
          )}
          {reasons.length > 0 && (
            <WhySection heading={t("worklist.why.now")}>
              <ol
                className="worklist-why-reasons"
                aria-label={t("worklist.why.reasons")}
              >
                {reasons.map((reason) => (
                  <li key={reason}>{reason}</li>
                ))}
              </ol>
            </WhySection>
          )}
          {consequence && (
            <WhySection heading={t("worklist.why.consequence")}>
              <p>{consequence}</p>
            </WhySection>
          )}
          {(open || answer) && (
            <WhySection heading={t("worklist.why.todo")}>
              {open && <p>{todoLine(item, t, told)}</p>}
              <OurLastReply item={item} />
              {answer && <RowVerbs item={item} />}
            </WhySection>
          )}
        </div>
      </PanelBody>
    </Panel>
  );
}

function WhySection({
  heading,
  children,
}: Readonly<{ heading: string; children: ReactNode }>) {
  return (
    <section className="worklist-why-section">
      <Eyebrow as="h3" size="xsmall">
        {heading}
      </Eyebrow>
      {children}
    </section>
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

// The rows whose told event is their due day or start, which their "overdue",
// "due today" or "starting soon" reason only restates.
const DATED_SOURCES = new Set<WorklistItem["source"]>([
  "task",
  "weekly_commitment",
  "meeting",
]);

// A reason with no words in this language is left out, not drawn blank.
// So is one that restates a date already told.
function whyReasons(
  item: WorklistItem,
  dated: boolean,
  t: Translator,
  locale: Locale,
  zone: string,
): string[] {
  // A reason said twice is read once, and the text is then its own key.
  const said = factReasons(item)
    .filter((reason) => !(dated && saidByTheWhenLine(reason.kind)))
    .map((reason) => reasonText(reason, t, locale, zone, partyName(item)))
    .filter((text): text is string => text !== null);
  return [...new Set(said)];
}
