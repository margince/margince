// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Why a row is on the list and what it asks, where its details open.
//
// A row says one current reason. Its details say what happened, every concrete
// fact behind it, what waiting costs and what to do. They leave out how the row
// ranks against the rest of the queue.

import { type ReactNode, useId } from "react";
import { Button } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatActivityWhen } from "../../format/activitywhen";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useSourceOpener } from "../../lib/opensource";
import { useViewerZone } from "../../lib/viewerzone";
import { saysOpen } from "../../lib/worklistask";
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
import { WebsiteInquiryFacts } from "./websiteinquiryfacts";
import { useBriefAnswer, withFoldedBrief } from "./worklist.briefverbs";
import { consequenceLine } from "./worklist.consequence";
import type { WorklistItem } from "./worklist.queries";
import { codedDetail } from "./worklist.reader";
import { inlineDecision, rowAnswer } from "./worklist.row.answer";
import { TaskExplanationSections } from "./worklist.taskexplanation";

import "./worklist.why.css";

const QUOTED_SOURCE = "conversation_claim";

export function WorklistWhy({
  item,
  answer = false,
  groundsShown = false,
  plain = false,
}: Readonly<{
  item: WorklistItem;
  // Draws the row's own verbs, for a drawer that covers the row it opened from.
  answer?: boolean;
  // The surface already reads the row's reason out, so it is not said twice.
  groundsShown?: boolean;
  // The pane's first section under its own heading, not a card inside it.
  plain?: boolean;
}>) {
  if (item.source === "task") {
    return <TaskWhy item={item} answer={answer} plain={plain} />;
  }
  return (
    <RowWhy
      item={item}
      answer={answer}
      groundsShown={groundsShown}
      plain={plain}
    />
  );
}

// A task is explained from its own records, so no template speaks for it.
function TaskWhy({
  item,
  answer,
  plain,
}: Readonly<{ item: WorklistItem; answer: boolean; plain: boolean }>) {
  const t = useT();
  const clock = useHappenedClock();
  // Nothing else here dates the task, so its due reasons stay.
  const reasons = whyReasons(item, false, t, clock.locale, clock.zone);
  const open = saysOpen(item);
  const subLevel = plain ? "h4" : "h3";
  return (
    <WhyFrame plain={plain}>
      <div className="worklist-why">
        {item.website_inquiry && (
          <WebsiteInquiryFacts facts={item.website_inquiry} />
        )}
        <TaskExplanationSections
          taskId={item.id}
          section={(heading, children) => (
            <WhySection level={subLevel} heading={heading}>
              {children}
            </WhySection>
          )}
          todo={
            (open || answer) && (
              <>
                {item.request_check && <p>{todoLine(item, t, false)}</p>}
                <OurLastReply item={item} />
                {answer && <RowVerbs item={item} />}
              </>
            )
          }
        />
        {reasons.length > 0 && (
          <WhySection level={subLevel} heading={t("worklist.why.now")}>
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
      </div>
    </WhyFrame>
  );
}

function RowWhy({
  item,
  answer,
  groundsShown,
  plain,
}: Readonly<{
  item: WorklistItem;
  answer: boolean;
  groundsShown: boolean;
  plain: boolean;
}>) {
  const t = useT();
  const clock = useHappenedClock();
  const { lines: happened, told } = happenedLines(item, t, clock, groundsShown);
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
  // Plain, the title is an h3 of its own, so each part sits a level under it.
  const subLevel = plain ? "h4" : "h3";
  const sections = (
    <div className="worklist-why">
      {(happened.length > 0 || detail) && (
        <WhySection level={subLevel} heading={t("worklist.why.happened")}>
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
      {item.website_inquiry && (
        <WebsiteInquiryFacts facts={item.website_inquiry} />
      )}
      {reasons.length > 0 && (
        <WhySection level={subLevel} heading={t("worklist.why.now")}>
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
        <WhySection level={subLevel} heading={t("worklist.why.consequence")}>
          <p>{consequence}</p>
        </WhySection>
      )}
      {(open || answer) && (
        <WhySection level={subLevel} heading={t("worklist.why.todo")}>
          {open && <p>{todoLine(item, t, told)}</p>}
          <OurLastReply item={item} />
          {answer && <RowVerbs item={item} />}
        </WhySection>
      )}
    </div>
  );
  return <WhyFrame plain={plain}>{sections}</WhyFrame>;
}

// The pane draws the reason as its first section; elsewhere it is a card.
function WhyFrame({
  plain,
  children,
}: Readonly<{ plain: boolean; children: ReactNode }>) {
  const t = useT();
  const headingId = useId();
  if (plain) {
    return (
      <section className="worklist-why-plain" aria-labelledby={headingId}>
        <Heading size="medium" as="h3" id={headingId}>
          {t("worklist.why.title")}
        </Heading>
        {children}
      </section>
    );
  }
  return (
    <Panel title={t("worklist.why.title")} tone="accent">
      <PanelBody>{children}</PanelBody>
    </Panel>
  );
}

function WhySection({
  heading,
  level,
  children,
}: Readonly<{ heading: string; level: "h3" | "h4"; children: ReactNode }>) {
  return (
    <section className="worklist-why-section">
      <Eyebrow as={level} size="xsmall">
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
