// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Badge } from "../../design-system/atoms";
import { PanelRow } from "../../design-system/panel";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { useMe } from "../../lib/session";
import { askKey } from "./worklist.ask";
import { useBriefAnswer, withFoldedBrief } from "./worklist.briefverbs";
import {
  briefContextTitle,
  briefSubjectLabel,
  dealFactsText,
  isUnprepared,
  itemTitle,
  phrasedReasons,
  reasonText,
  rowHref,
  subjectHref,
  whenText,
} from "./worklist.copy";
import { DepartedOwnerCaption } from "./worklist.departedowner";
import { PutDownByThumb } from "./worklist.dispositions";
import { WaitingEmailLine } from "./worklist.emailtitle";
import { conditionOf, eyebrowKeyFor, kindClass } from "./worklist.eyebrow";
import { leadFactsText } from "./worklist.leadfacts";
import { WorklistMayBeDone } from "./worklist.maybedone";
import { hostText } from "./worklist.meetingparties";
import { useRowSource } from "./worklist.noticesource";
import { lastTouch } from "./worklist.pane";
import type { WorklistItem } from "./worklist.queries";
import {
  noticeDetail,
  readerTask,
  sharedResponsibility,
} from "./worklist.reader";
import { PromiseRecipients } from "./worklist.recipients";
import {
  aboutRecord,
  shapedReadings,
  touchOf,
  useListed,
} from "./worklist.row.captions";
import { CompactRowLine, type RowReadings } from "./worklist.row.compact";
import { RowActs } from "./worklist.rowverbs";
import "./worklist.row.css";
import { PreparationNote } from "./worklist.ready";
import { rowAnswer } from "./worklist.row.answer";
import { Rank, RowLead, RowText, titleLinks } from "./worklist.row.cells";

// Preserve server order while naming each group member once.
function namedMembers(item: WorklistItem): string[] {
  return [...new Set(item.batch?.sample ?? [])];
}

/**
 * How much of the row is drawn, and what each density owes.
 *
 * A DISCRIMINATED PAIR rather than one optional `position`, because the rank is
 * required by the default density and refused by the compact one — a plain
 * optional would let a Worklist caller forget it and lose the digit silently,
 * which is the shape of failure a gate cannot see. This one the compiler sees.
 */
type RowDensity =
  | Readonly<{
      density?: undefined;
      /** Where this row sits in the day's order, drawn as the rank. */
      position: number;
    }>
  | Readonly<{
      /**
       * ONE LINE per row: no rank, the title carrying the link, the captions
       * beside it, and everything else one press away
       * (worklist.row.compact.tsx). For a surface that OPENS with a prefix of
       * the queue and goes on to something else.
       */
      density: "compact";
      /** Refused: the rank is a column this density does not draw. */
      position?: undefined;
    }>;

export function WorklistRow({
  item,
  position,
  density,
  owner,
  allowPin = true,
  selected,
  onSelect,
  onReview,
  onOpenEmail,
  context,
  framed = false,
  pick,
}: Readonly<{
  item: WorklistItem;
  /** The row's selection checkbox, drawn beside its rank where the list
   *  offers a bulk verb over the row. */
  pick?: ReactNode;
  /** The way into what this row is ABOUT, drawn among its verbs. The Brief
   *  has no pane beside its list, so its focus rows open a drawer instead. */
  context?: ReactNode;
  /** A CARD around this row says whose it is, why it is here and where it
   *  stands, so the row withholds those captions and the verb that only
   *  reaches its record. The Brief's card is the only frame there is. */
  framed?: boolean;
  // Whose queue this row is on, empty for the reader's own. It names the
  // contact a reassignment moves work away from. On the reader's own queue
  // that is the reader, whom ReassignForm resolves, so empty is a real state.
  owner: string;
  /** Personal ordering is available on the queue, not the Focus projection. */
  allowPin?: boolean;
  // Whether the pane beside the queue is about this row.
  //
  // BOTH CALLBACKS ARE OPTIONAL, because one surface has no pane. The Brief
  // shows the head of this same queue on the page a rep opens first, and it has
  // no second column to open a row INTO — so passing a no-op would draw a rank
  // button that answers nothing, which is exactly the dead control the comment
  // on that button warns about. Absent means the affordance is not drawn and
  // the rank reads as the number it is.
  selected?: boolean;
  onSelect?: () => void;
  // Where a grouped row is reviewed. Also a filter change the Brief cannot
  // make: it shows a fixed cut and has no filter to move.
  onReview?: () => void;
  // Opens a waiting email. Unlike the pane above, this one is carried by every
  // surface that draws a waiting row, the Brief included: a drawer needs no
  // second column, only a dialog, and the row draws the whole message —
  // sender, subject, preview, access badge. A row that shows a reader the
  // message and refuses to open it teaches them the product does not work.
  // Optional only for a caller that draws no waiting row at all.
  onOpenEmail?: (activityId: string) => void;
}> &
  RowDensity) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  // A task's deadline is the record's day; everything else on this row is a
  // moment the reader is racing on their own clock.
  const recordZone = useRecordZone();
  // Inside the drawer a row's record links carry the way back to it.
  const listed = useListed();
  const target = rowHref(item);
  const href = target && listed(target);
  // Open the activity itself without leaving the queue for its linked record.
  const source = useRowSource(item);
  const viewer = useMe(false).data?.user;
  const name = framedName(readerTask(item, viewer, t), t, locale, framed);
  const title = name.title;
  const facts =
    dealFactsText(item, t, locale, zone) ??
    leadFactsText(item, t, locale, zone);
  const sample = namedMembers(item);
  // The clock this row is racing. A meeting said "starting shortly" whether it
  // began in four minutes or in fifty, and a task said "Overdue" without saying
  // by how long — on the two rows whose whole claim is a moment.
  const when = whenText(item, t, locale, zone, recordZone, new Date());
  // The supporting line, which every source sends as a sentence already.
  const sourceDetail = noticeDetail(item, viewer, t);
  const detail = sourceDetail === item.verdict?.line ? undefined : sourceDetail;
  // The badged reasons are drawn as badges above and left out here, so one
  // meeting does not report the same finding twice in two registers. The when
  // line takes `due_today` the same way when it is drawn: the moment names the
  // hour a rep is racing, and "due today" underneath it is that clock said
  // again in a coarser register.
  const reasons = phrasedReasons(item, when !== null)
    .map((reason) => reasonText(reason, t, locale, zone))
    .filter((phrase): phrase is string => phrase !== null);
  const said = [...new Set(reasons)].filter(
    (reason) => reason !== detail && reason !== item.verdict?.line,
  );
  // How this row NAMES ITSELF: the canonical email row when there is a message
  // AND somewhere to open it, the title line otherwise. Held as the opener
  // rather than as a flag, so the row cannot be drawn without one — a caller
  // with no drawer keeps the title instead of losing the row's name with it.
  const emailOpener = item.email_summary != null ? onOpenEmail : undefined;
  // WHICH RECORD THE ROW IS ABOUT, linked, on the one row that does not link
  // it: a waiting message names its sender as text, where every other title
  // names and links its record (`itemTitle`, `rowHref`). A FRAMED row names it
  // under the row itself (brief.focus.tsx) and withholds this — the frame, not
  // the verb order, because the drawer takes that order with no card around
  // its rows and this line is the only place those facts are said there.
  const about = framed
    ? undefined
    : aboutRecord(item, emailOpener !== undefined, listed);
  const touch = lastTouch(touchOf(item, framed), t, locale, zone);
  const host = hostText(item, viewer?.id, t);
  const responsibility = sharedResponsibility(item, locale, t);
  // Whether the day put a state on this row — overdue, or a meeting with
  // nothing prepared. They ride on the title line, which is why it is drawn on
  // a row that has no title of its own to draw.
  const badged = item.overdue === true || isUnprepared(item);
  // One brief write per row, shared by Act and the two set-asides. The hook
  // registers a mutation and reads nothing, so every row may call it. Two
  // writes would carry two settled flags, and one item could be answered twice.
  const brief = useBriefAnswer(item);
  const answer = withFoldedBrief(rowAnswer(item, brief), item, brief);
  const readings: RowReadings = shapedReadings(framed, {
    item,
    ...name,
    href,
    open: source.open,
    sourceUnavailable: source.unavailable,
    when,
    facts,
    said,
    detail,
    sample,
    zone,
    about,
    host,
    touch,
  });
  const named = conditionOf(item, t);
  return (
    <PanelRow
      // A CLASS and not a data attribute: `PanelRow` takes a className and
      // forwards nothing else, and reaching into that primitive to pass one
      // attribute through would make a screen the second author of the house
      // row.
      className={[
        "worklist-row",
        selected ? "worklist-row-selected" : "",
        density === "compact" ? "worklist-row-compact" : "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {/* Below the fold the row itself answers the set-aside judgements, whose
          verbs do not fit beside the work at 390px. It wraps the row rather
          than the verbs because the row is what a thumb lands on; above the
          fold it draws its children and nothing else. */}
      <PutDownByThumb item={item}>
        {/* THE RANK IS WHAT THE DENSITY GIVES UP FIRST. It is a claim about
            order, and at one line per row the ordered list already carries
            that claim — so a digit per row spends a column saying again what
            the page says once. `position` is refused in compact rather than
            ignored: see RowDensity. */}
        <RowLead pick={pick}>
          {position !== undefined && (
            <Rank
              position={position}
              title={title}
              selected={selected}
              onSelect={onSelect}
            />
          )}
        </RowLead>
        {/* WHAT KIND of work, in its own column at a width that has one, so a
            reader running down the queue reads the kinds as a list without
            reading a title first — and in the warning tone on the rows the day
            put in its first band, where the kind is also why it is there. The
            title line keeps the states that are about this row alone: overdue,
            unprepared.

            A soft `Badge`, the variant a column carrying one per row wears:
            the solid fill is kept for the one status a surface must not let a
            reader miss, and a queue of those would teach the eye to skip them.
            The span is PLACEMENT — the grid cell and the
            width the kinds share; `conditionOf` says what a system row
            draws there instead. */}
        <span className={kindClass(named)} title={named ?? undefined}>
          <Badge tone={item.band === "now" ? "warning" : undefined}>
            {named ?? t(eyebrowKeyFor(item))}
          </Badge>
        </span>
        <div className="worklist-row-text">
          {/* A waiting EMAIL names itself with the canonical row — the same one
            the timeline draws — so the queue shows the message rather than a
            sentence about it. Everything else keeps the title line it had, and
            the badges below stay on both: they say where the row sits in the
            day, which the email row does not answer. */}
          {emailOpener && <WaitingEmailLine item={item} onOpen={emailOpener} />}
          {/* Not drawn EMPTY. An email row is named by the message above, so
              its title line held nothing at all unless the day had a state to
              put on it — and an empty line still collects the text column's
              interval, which is a stranded gap between the message and the
              facts under it on every waiting row. */}
          {density === "compact" ? (
            // ONE LINE, off the same readings the default column prints. The
            // name is withheld where the message above already carries it.
            <CompactRowLine readings={readings} named={!emailOpener} />
          ) : (
            <RowText
              readings={readings}
              emailOpener={emailOpener}
              badged={badged}
            />
          )}
          <RowAsk item={item} framed={framed} />
          <PreparationNote item={item} />
          {responsibility && <p className="t-caption">{responsibility}</p>}
          <DepartedOwnerCaption owner={item.owner} />
          <PromiseRecipients recipients={item.promise_recipients} />
          <WorklistMayBeDone item={item} />
        </div>
        {/* Every verb on one right-aligned line, the row's menu last. Under the
            work where the card has no column to spare for it, beside the work
            where it has, so the answer keeps one x down the whole queue. */}
        <RowActs
          allowPin={allowPin}
          onOpenEmail={onOpenEmail}
          item={item}
          title={title}
          href={href}
          owner={owner}
          primary={answer.primary}
          equals={answer.equals}
          onReview={onReview}
          context={context}
          framed={framed}
          titled={titleLinks(emailOpener, href, source.open)}
        />
        {/* An answer that is not a VERB: a duplicate pair, whose two buttons
            each name the record they keep and cannot leave the list that names
            it. It stays a block under the row, where it has the width to show
            both sides. */}
        {answer.below}
        {source.drawer}
      </PutDownByThumb>
    </PanelRow>
  );
}

function framedName(
  item: WorklistItem,
  t: Parameters<typeof itemTitle>[1],
  locale: Parameters<typeof itemTitle>[2],
  framed: boolean,
) {
  const subject = framed ? briefSubjectLabel(item) : undefined;
  const subjectNamed =
    framed &&
    !!subjectHref(item) &&
    (!item.contact?.label || item.contact.id === item.subject?.id);
  return {
    title: subject ?? itemTitle(item, t, locale, subjectNamed),
    heading: subject !== undefined,
    supportingTitle: subject ? briefContextTitle(item) : undefined,
  };
}

// The card is read on its own, so it says what the row asks. A queue row leaves
// that to the details its reader opens.
function RowAsk({
  item,
  framed,
}: Readonly<{ item: WorklistItem; framed: boolean }>) {
  const t = useT();
  return framed ? <p className="t-body">{t(askKey(item))}</p> : null;
}
