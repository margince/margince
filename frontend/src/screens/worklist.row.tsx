// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One row of the day: how a piece of work reads, and the answers it carries.
//
// Split from the screen because they answer different questions. The screen
// decides WHAT the page shows — whose day, which cut, which headings. A row
// decides how one piece of work reads, and that is the half a reader of either
// question does not need the other for. The line of verbs under it is one more
// step down, in worklist.rowverbs.tsx.

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useRef, useState } from "react";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button, Modal } from "../design-system/atoms";
import { PanelRow } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { translatePlural, useLocale, useT } from "../i18n";
import { ApprovalRow } from "./approvalrow";
import { type BriefMarkRequest, useBriefItemMark } from "./brief.queries";
import { tomorrowMorning } from "./briefqueue";
import { problemMessageOf } from "./common";
import { ChannelReplyAction, RELINK_KINDS, type RelinkKind } from "./compose";
import { hasMoveControl, MoveButton } from "./movebutton";
import {
  useAutomationRetry,
  useClaimSettle,
  useMeetingOutcome,
  useNoticeRead,
  useTaskUpdate,
} from "./taskactions";
import {
  comparisonText,
  consequenceText,
  dealFactsText,
  isUnprepared,
  itemTitle,
  phrasedReasons,
  reasonText,
  rowHref,
  whenText,
} from "./worklist.copy";
import { PutDownByThumb } from "./worklist.dispositions";
import { WaitingEmailLine } from "./worklist.emailtitle";
import { PairDecision } from "./worklist.pair";
import {
  useApproval,
  useNudgeDismissal,
  type WorklistItem,
  worklistKey,
} from "./worklist.queries";
import { RowActs } from "./worklist.rowverbs";
import { syncHealthDetail } from "./worklist.synchealth";
import { VerdictLine } from "./worklist.verdict";
import "./worklist.row.css";

/**
 * A grouped row's named members, each ONCE.
 *
 * The contract asks for "a few members, named, so the group can be checked
 * before it is answered", and a group of eight failures of one automation sends
 * that automation's name eight times: the Worklist's top row read
 * "Post-meeting recap draft · Post-meeting recap draft · Post-meeting recap
 * draft", which tells a reader nothing about the group except that the list
 * repeats.
 *
 * Order is kept — first appearance wins — because the server sends them in the
 * order it thinks matters.
 */
function namedMembers(item: WorklistItem): string[] {
  return [...new Set(item.batch?.sample ?? [])];
}

export function WorklistRow({
  item,
  position,
  owner,
  selected,
  onSelect,
  onReview,
  onOpenEmail,
}: Readonly<{
  item: WorklistItem;
  position: number;
  // Whose queue this row is on, empty for the reader's own. It names the
  // person a reassignment moves work AWAY from, which on the reader's own
  // queue is the reader — ReassignControl resolves that rather than this
  // prop carrying it, so an empty value is a real state and not a missing one.
  owner: string;
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
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  // A task's deadline is the record's day; everything else on this row is a
  // moment the reader is racing on their own clock.
  const recordZone = useRecordZone();
  const href = rowHref(item);
  const title = itemTitle(item, t, locale);
  const facts = dealFactsText(item, t, locale, zone);
  const sample = namedMembers(item);
  // The clock this row is racing. A meeting said "starting shortly" whether it
  // began in four minutes or in fifty, and a task said "Overdue" without saying
  // by how long — on the two rows whose whole claim is a moment.
  const when = whenText(item, t, locale, zone, recordZone, new Date());
  // The supporting line. Every source but one sends a sentence already;
  // sync_health sends its condition's facts in its own vocabulary, so its line
  // is written from `kind` and `detail` together.
  const detail =
    item.source === "sync_health"
      ? syncHealthDetail(item.kind, item.detail, t)
      : item.detail;
  // The badged reasons are drawn as badges above and left out here, so one
  // meeting does not report the same finding twice in two registers. The when
  // line takes `due_today` the same way when it is drawn: the moment names the
  // hour a rep is racing, and "due today" underneath it is that clock said
  // again in a coarser register.
  const reasons = phrasedReasons(item, when !== null)
    .map((reason) => reasonText(reason, t, locale, zone))
    .filter((phrase): phrase is string => phrase !== null);
  const above = comparisonText(item.above_next, t, locale, zone);
  const consequence = consequenceText(item, t);
  // How this row NAMES ITSELF: the canonical email row when there is a message
  // AND somewhere to open it, the title line otherwise. Held as the opener
  // rather than as a flag, so the row cannot be drawn without one — a caller
  // with no drawer keeps the title instead of losing the row's name with it.
  const emailOpener = item.email_summary != null ? onOpenEmail : undefined;
  // Whether the day put a state on this row — overdue, or a meeting with
  // nothing prepared. They ride on the title line, which is why it is drawn on
  // a row that has no title of its own to draw.
  const badged = item.overdue === true || isUnprepared(item);
  // WHERE this lane's answer sits, and the write a brief item's two placements
  // share. The hook is called on every row and fires on none it is not asked
  // to: `useBriefItemMark` registers a mutation and reads nothing, and holding
  // it here is what lets "Act" lead the row's verbs while the two set-asides
  // stand among the quieter ones WITHOUT the row growing a second mutation.
  // Two of them would each carry their own settled flag, so acting and then
  // dismissing would answer one item twice — the same reason PutDownByThumb
  // holds the disposition write for the verbs and the swipe.
  const brief = useBriefAnswer(item);
  const answer = rowAnswer(item, brief);
  return (
    <PanelRow
      className={
        selected ? "worklist-row worklist-row-selected" : "worklist-row"
      }
    >
      {/* Below the fold the row itself answers the set-aside judgements, whose
          verbs do not fit beside the work at 390px. It wraps the row rather
          than the verbs because the row is what a thumb lands on; above the
          fold it draws its children and nothing else. */}
      <PutDownByThumb item={item}>
        <Rank
          position={position}
          title={title}
          selected={selected}
          onSelect={onSelect}
        />
        {/* WHAT KIND of work, in its own column at a width that has one, so a
            reader running down the queue reads the kinds as a list without
            reading a title first — and in the warn tone on the rows the day
            put in its first band, where the kind is also why it is there. The
            title line keeps the states that are about this row alone: overdue,
            unprepared.

            `Badge quiet` because that is the catalog's answer for a column
            carrying one per row: the tone survives as a dot and the label as
            plain text, where a filled pill down a queue reads as decoration a
            reader learns to skip. The span is PLACEMENT — the grid cell and the
            width the kinds share — and draws nothing itself. */}
        <span className="worklist-row-kind">
          <Badge quiet tone={item.band === "now" ? "warn" : undefined}>
            {t(`worklist.category.${item.category}` as const)}
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
          {(!emailOpener || badged) && (
            <p className="t-body worklist-row-title">
              {emailOpener ? null : href ? (
                <a className="entity-link" href={href}>
                  {title}
                </a>
              ) : (
                title
              )}
              {item.overdue && (
                <Badge tone="danger">{t("worklist.overdue")}</Badge>
              )}
              {/* A state of the meeting, not a reason among reasons: a rep
                scanning for the one to open before it starts has to see it
                without reading the line under the title. Warn rather than
                danger — an unprepared meeting is work to do, not a deadline
                already missed. */}
              {isUnprepared(item) && (
                <Badge tone="warn">{t("worklist.needsPrep")}</Badge>
              )}
            </p>
          )}
          {/* The supporting line, from every source that sends PROSE.

            It was drawn for `notice` alone, because three sources used this
            field as a typed channel — two wrote a bare day count, one wrote the
            marker words the queue groups by — and drawing it would have printed
            "90" under one title and "machine_sender" under another. Those three
            send their values typed now, so the twelve sources that were already
            writing sentences get to say them: which mailbox stopped, why a
            message bounced, why a send was held, what an AI task was about,
            which rule failed and how. That is the decisive line on most of these
            rows, and a reader was reading around it.

            `sync_health` sends its facts in the producer's own vocabulary —
            `shed`, `rate_limited`, `deals, contacts` — so its line is WRITTEN
            from that pair rather than drawn, by worklist.synchealth.ts. A value
            that build does not recognise draws nothing, which is what this row
            did for every sync value before. */}
          {detail && <p className="t-caption worklist-row-detail">{detail}</p>}
          {sample.length > 0 && (
            // A group nobody can see into is a group nobody trusts, and an
            // untrusted group is worse than the pile it replaced.
            <p className="t-caption worklist-row-sample">
              {sample.join(" · ")}
            </p>
          )}
          {/* How the deal is standing, above the captions rather than among
              them. It is a READING and they are facts, and a reader who cannot
              tell those apart cannot tell what to trust — worklist.verdict.tsx
              states why the label says which. */}
          <VerdictLine verdict={item.verdict} zone={zone} />
          <RowCaptions
            when={when}
            facts={facts}
            reasons={reasons}
            consequence={consequence}
            above={above}
          />
        </div>
        {/* EVERY VERB ON ONE RIGHT-ALIGNED LINE, the lane's answer LAST. Under
            the work where the card has no column to spare for it, beside the
            work where it has, and on the trailing edge in both — so the answer
            keeps one x down the whole queue. worklist.rowverbs.tsx states why
            it is the tail of the line rather than its head. */}
        <RowActs
          item={item}
          href={href}
          owner={owner}
          primary={answer.primary}
          equals={answer.equals}
          onReview={onReview}
        />
        {/* An answer that is not a VERB: a duplicate pair, whose two buttons
            each name the record they keep and cannot leave the list that names
            it. It stays a block under the row, where it has the width to show
            both sides. */}
        {answer.below}
      </PutDownByThumb>
    </PanelRow>
  );
}

/**
 * The answer a row can carry INSIDE it, and WHERE in the row it goes.
 *
 * Three kinds, and what they share is the reason they are here rather than
 * behind a link: the server already sent everything the decision needs, so
 * routing the reader to another screen to make it would be the hand-off this
 * queue exists to remove. An approval carries its staged payload, a duplicate
 * carries both records, a notice needs only to be seen.
 *
 * Together rather than inline in the row, because the row's own job — rank,
 * title, reasons, verbs — is already at the complexity the linter allows, and
 * a fourth kind of answer should extend this list rather than that function.
 */

/**
 * The three places an answer can stand, and each lane picks one.
 *
 * A PLACEMENT rather than a node, because "the answer" is not one shape. Most
 * lanes have a single call to action, and it ends the row's verbs. Some offer
 * several verbs of EQUAL weight, and there a call to action is a lie — it would
 * name one of three meeting outcomes as the expected one. And one carries a
 * payload rather than a verb at all.
 *
 * All three optional, and a lane with nothing to answer returns none of them: a
 * row the server named no verb on is still real work with nothing to press.
 */
type RowPlacement = Readonly<{
  /** The lane's one call to action, at the trailing end of the row's verbs. */
  primary?: ReactNode;
  /** Verbs of equal weight, among the row's quieter ones. */
  equals?: ReactNode;
  /** An answer carrying its own layout, under the row rather than in it. */
  below?: ReactNode;
}>;
// The answers the row itself is enough for, keyed by the source that carries
// them and the verb the server sent. A table rather than a branch each: they
// differ only in which control to draw, so spelling them as code made `rowAnswer`
// grow one arm per source until it hit the complexity ceiling — which its own
// doc predicted.
//
// Both halves of the key matter. The SOURCE decides the control, and the VERB
// is the server's judgement that this particular row may use it: a blocked
// automation carries no `retry`, so it draws nothing here.
//
// `draw` takes the ROW, not its id. A task's completion is pinned to the
// version the reader decided on, and every other verb here writes too — so the
// entry that needs a second field next is served without leaving the table for
// a branch of its own, which is how `rowAnswer` grew arms the last time.
const ANSWER_BY_SOURCE: Partial<
  Record<
    WorklistItem["source"],
    {
      verb: WorklistItem["actions"][number];
      draw: (item: WorklistItem) => RowPlacement;
    }
  >
> = {
  notice: {
    verb: "acknowledge",
    draw: (item) => ({ primary: <NoticeAcknowledge id={item.id} /> }),
  },
  automation_run: {
    verb: "retry",
    draw: (item) => ({ primary: <AutomationRetry id={item.id} /> }),
  },
  // THREE verbs of equal weight, so the row has no primary. Held, no-show and
  // cancelled are equally likely records of what already happened, and leading
  // the row with one of them would read as the product's expectation about a
  // meeting it knows nothing about.
  meeting_outcome: {
    verb: "decide",
    draw: (item) => ({
      equals: <MeetingOutcome id={item.id} version={item.version} />,
    }),
  },
  conversation_claim: {
    verb: "complete",
    draw: (item) => ({ primary: <PromiseKept id={item.id} /> }),
  },
  task: {
    verb: "complete",
    draw: (item) => ({
      primary: <TaskComplete id={item.id} version={item.version} />,
    }),
  },
  // The row's id IS the person's here, which is what the dismissal endpoint
  // takes — the pairing is why this verb is offered on this lane and nowhere
  // else. `dismiss` also belongs to brief_item, where it means something else
  // and posts somewhere else, which is why this table is keyed by SOURCE.
  relationship_decay: {
    verb: "dismiss",
    draw: (item) => ({ primary: <NudgeDismiss personId={item.id} /> }),
  },
};

function rowAnswer(item: WorklistItem, brief: BriefAnswer): RowPlacement {
  if (decidable(item)) {
    return { primary: <RowDecision item={item} /> };
  }
  if (item.source === "dedupe_candidate" && item.pair) {
    // UNDER the row, not in it. Each of its two verbs names the record it
    // would keep and stands in the list entry that describes that record —
    // lifted out into a row of verbs, "Keep Acme GmbH" and "Keep Acme GmbH"
    // would be two identical buttons over an irreversible merge.
    return { below: <PairDecision item={item} /> };
  }
  // Never a BATCH: a group row stands for a pile and names no single record, so
  // every id-keyed answer below would act on the wrong one.
  const keyed = ANSWER_BY_SOURCE[item.source];
  if (keyed && !item.batch && item.actions.includes(keyed.verb)) {
    return keyed.draw(item);
  }
  // Its own branch, because it is the one answer that needs more than the row's
  // id: the composer files the sent message against a record, so the subject
  // travels with it.
  const replyTo = replyTarget(item);
  if (replyTo) {
    return { primary: <WaitingReply id={item.id} to={replyTo} /> };
  }
  // A brief item's three verbs, RANKED the way the row ranks them: acting on
  // the day's pick is what the reader came for, and setting it aside or
  // dismissing it are the two ways of declining. All three share one write.
  if (item.source === "brief_item" && !item.batch) {
    return {
      primary: brief.offered("act") ? (
        <BriefAct item={item} brief={brief} />
      ) : undefined,
      equals: <BriefSetAsides item={item} brief={brief} />,
    };
  }
  // The one decided step a link cannot take.
  //
  // Every other move the server sends already reaches the reader, as an anchor
  // through moveHref — draft_reply and draft_email open the composer,
  // open_task and open_meeting_brief open what they name. `create_task` POSTS a
  // task body, which is a write and not a destination, so NAVIGABLE_MOVES
  // excludes it and the row could name the step and offer no way to take it.
  //
  // The button is the deal status card's own, mounted a second time rather than
  // written again: one answer to "what does Add this task do", on the two
  // surfaces that draw the same move.
  //
  // LAST, and after brief_item deliberately. A brief item carries a deal
  // subject, and the backend attaches a cached move to any deal-subject row
  // that has none — so an earlier position here would replace Act, Set aside
  // and Dismiss with a task button on a row whose own verbs are the point.
  if (item.move?.action === "create_task" && hasMoveControl(item.move)) {
    return {
      primary: (
        <MoveButton
          dealId={item.subject?.type === "deal" ? item.subject.id : undefined}
          move={item.move}
        />
      ),
    };
  }
  return {};
}

// Setting a lapsed contact aside for a month.
//
// Nobody is waiting on a quiet contact, which is exactly why the row kept
// coming back: there was no way to say "not this one, not now", so a rep who
// had already decided met the same person every morning.
//
// UNDOABLE from the confirmation, like every disposition beside it. The row
// leaves the lane on success, so a misclick otherwise costs the reader the only
// address they had for a contact they were not done with.
function NudgeDismiss({ personId }: Readonly<{ personId: string }>) {
  const t = useT();
  const toast = useToast();
  const { dismiss, restore } = useNudgeDismissal();
  return (
    <Button
      small
      pending={dismiss.isPending}
      onClick={() =>
        dismiss.mutate(
          { personId },
          {
            onSuccess: () =>
              toast.show(t("worklist.verb.dismissed"), {
                action: {
                  label: t("worklist.verb.dismissUndo"),
                  // The toast dismisses itself the moment the action is
                  // pressed, so a failed undo leaves the contact set aside
                  // with the only way back already off the screen.
                  //
                  // mutateAsync and a catch, for the reason TaskComplete
                  // gives above: the dismissal REMOVES the row, so by the
                  // time the reader presses Undo this component is unmounted
                  // and React Query has dropped the observer that per-call
                  // callbacks hang off. A refused undo would say nothing at
                  // all — the reader presses the one control that undoes
                  // their misclick, it fails, and the screen is silent.
                  onAct: () => {
                    restore.mutateAsync({ personId }).catch(() =>
                      toast.show(t("worklist.verb.dismissUndoFailed"), {
                        mark: false,
                      }),
                    );
                  },
                },
              }),
            onError: () =>
              toast.show(t("worklist.verb.dismissFailed"), { mark: false }),
          },
        )
      }
    >
      {t("worklist.verb.dismiss")}
    </Button>
  );
}

/**
 * How many reasons a row says before the rest go behind a tap.
 *
 * A COUNT, because the ceiling has to survive the vocabulary growing. Saying
 * only what a row contains today puts it back over the limit the next time
 * somebody adds a reason, and that person has no way to know they did.
 *
 * Three because three still fit on ONE line at 390px. Measured 2026-09-05:
 * two reasons and three are both 19px; the fourth wraps to 37px and the sixth
 * to 56px. So the fold costs a reader nothing until the line would have taken
 * a second line anyway.
 */
const REASONS_BEFORE_THE_FOLD = 3;

/**
 * Why this row is here — the first few said outright, the rest a tap away.
 *
 * NOTHING IS DISCARDED, which is the whole shape of this. A cap that dropped
 * the overflow was tried and abandoned (it dropped the wrong ones: `pinned`,
 * `expected_revenue` and an absorbed deal's grounds are all appended LAST
 * because they are applied late, so a head-of-list cut takes exactly the facts
 * that decided where the row sits). The reasons arrive "in the order they were
 * weighed", so the first ones are the strongest and the fold falls in the
 * right place by construction — but the rest stay reachable rather than being
 * silenced.
 *
 * The same shape the deal status card uses: first line out, remainder behind a
 * disclosure. One answer to "too many reasons", not a second one written here.
 *
 * The summary NAMES THE COUNT rather than saying "more". A reader deciding
 * whether to spend a tap wants to know if it is one more fact or four.
 *
 * WHY THIS ROW BEAT THE ONE BELOW IT is folded here too, and that is the whole
 * of where it lives now. It is one sentence of the same subject — why this row
 * is where it is — and it was drawn as a permanent third caption line under
 * every row on the page, which is a full line of height spent on a comparison
 * nobody reads until they disagree with the order. Behind this press it is one
 * tap away from the reader who does. It goes LAST and never into the summary:
 * the reasons are fragments of a dozen characters and this is a full sentence
 * with two timestamps in it, so on the line it would take the second line the
 * fold exists to save. The count covers it, because the count is what a reader
 * spends the tap on — one that named only the reasons would promise less than
 * the fold holds.
 */
function RowWhyHere({
  reasons,
  above,
}: Readonly<{ reasons: readonly string[]; above: string | null }>) {
  const t = useT();
  const { locale } = useLocale();
  const said = reasons.slice(0, REASONS_BEFORE_THE_FOLD);
  const folded = reasons.slice(REASONS_BEFORE_THE_FOLD);
  const behind = folded.length + (above ? 1 : 0);
  if (behind === 0) {
    return said.length === 0 ? null : (
      <p className="t-caption worklist-row-because">{said.join(" · ")}</p>
    );
  }
  return (
    <details className="worklist-row-because-fold">
      <summary className="t-caption worklist-row-because">
        {said.length === 0 ? (
          // Nothing is said on the line, so there is no "more" to count from
          // and the fold names itself instead — in the words the product
          // already has for this question.
          <span className="worklist-row-because-more">
            {t("worklist.verdict.rule")}
          </span>
        ) : (
          <>
            {said.join(" · ")}{" "}
            <span className="worklist-row-because-more">
              {translatePlural(locale, "worklist.because.more", behind, {
                // The reader's own notation, not String(): a count drawn for a
                // person goes through the formatter like every other magnitude,
                // and jsx-magnitude.test.ts holds that for the whole tree.
                count: formatNumber(behind, locale),
              })}
            </span>
          </>
        )}
      </summary>
      {folded.length > 0 && (
        <p className="t-caption worklist-row-because">{folded.join(" · ")}</p>
      )}
      {above && <p className="t-caption worklist-row-above">{above}</p>}
    </details>
  );
}

/**
 * Everything the row says about itself under the title, in the order a reader
 * needs it.
 *
 * When it happens, what it is worth, why it is ranked where it is, and what
 * doing nothing costs. Each is absent when the server sent nothing for it — a
 * caption drawn empty is a line of furniture the reader has to look past on
 * every row.
 *
 * TWO LINES AND NOT FIVE. The row printed the moment, the figures, the reasons,
 * the consequence and the comparison as five stacked captions, which is five
 * lines of 12px grey under every title on the page — and a reader scanning a
 * queue reads the first of them and skips the rest. The facts that are
 * FRAGMENTS ("due 15:00", "€40k", "waiting 4 days · nobody owns it") now read
 * as one dot-separated line at every width, in the same order and still as
 * separate elements, so a screen reader still meets three facts and only the
 * line breaks between them go. What keeps a line of its own is the sentence a
 * reader is meant to stop at: what it costs to do nothing.
 *
 * Together in one component because they are one idea — the row's own account
 * of itself — and because the row's function had reached the complexity the
 * linter allows, which is a fair reading of how much a person can hold at once.
 */
function RowCaptions({
  when,
  facts,
  reasons,
  consequence,
  above,
}: Readonly<{
  when: string | null;
  facts: string | null;
  reasons: readonly string[];
  consequence: string | null;
  above: string | null;
}>) {
  return (
    <>
      {/* When it starts, or when it is due, FIRST — it is the fact the reasons
          beside it are about: "starting shortly" explains a rank, and this says
          what time. */}
      <div className="worklist-row-facts-line">
        {when && <p className="t-caption worklist-row-when">{when}</p>}
        {facts && <p className="t-caption worklist-row-facts">{facts}</p>}
        <RowWhyHere reasons={reasons} above={above} />
      </div>
      {/* What it costs to do nothing. The question a queue exists to answer,
          and the one the lane feed had no field for. */}
      {consequence && (
        <p className="t-caption worklist-row-consequence">{consequence}</p>
      )}
    </>
  );
}

/**
 * The rank, and the way INTO the pane where there is one.
 *
 * A button on the rank rather than the whole row being pressable: the row
 * already holds links and verbs, and a control wrapping controls is a press
 * whose target the reader has to guess.
 *
 * Without `onSelect` it is a plain number. The Brief draws these rows on a page
 * with no second column, so a button there would open nothing — and a control
 * that answers nothing is worse than no control, because a reader presses it
 * once and learns the page lies about what is pressable.
 */
function Rank({
  position,
  title,
  selected,
  onSelect,
}: Readonly<{
  position: number;
  title: string;
  selected?: boolean;
  onSelect?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The rank is the page's central claim, so it is readable rather than
  // decorative: the list element carries the order for a screen reader and the
  // number states it for everybody else.
  const digit = (
    <span className="t-caption worklist-rank">
      {formatNumber(position, locale)}
    </span>
  );
  if (!onSelect) {
    return digit;
  }
  return (
    <button
      type="button"
      className="worklist-rank-select"
      aria-pressed={selected ?? false}
      // Names the ROW, not the verb. Every rank button carrying the same label
      // made them indistinguishable to anybody navigating by name, and overrode
      // the visible digit — which is the one thing that told them apart on
      // screen.
      aria-label={t("worklist.pane.openRow", {
        position: formatNumber(position, locale),
        title,
      })}
      onClick={onSelect}
    >
      {digit}
    </button>
  );
}

// Whether this row is a decision a person answers HERE.
//
// The queue holds no authority of its own — the card below is the same one the
// record page draws, posting to the same endpoint. What the queue adds is that
// the decision is answerable where it was ranked, instead of sending a reader
// to a second screen to do what the row already described.
function decidable(item: WorklistItem): boolean {
  return item.actions.includes("decide") && item.source === "approval";
}

// The decision itself, fetched whole because a row cannot carry it — and
// answered in a DRAWER rather than in the row.
//
// It used to render inline, and that is what made the queue unusable on a
// phone. The card carries evidence, a draft and three answers; measured at
// 390x844 it stood 440px tall inside a row whose ceiling is 208, which pushed
// the first primary action of the whole page to 920px down an 844px screen.
// The reader had to scroll past one decision to reach the work.
//
// The row keeps the decision's SUMMARY and one button. The drawer holds the
// card — the same ApprovalRow the record page draws, posting to the same
// endpoint — so the queue still adds no authority of its own. What it adds is
// that the decision is answerable where it was ranked.
//
// Held by: AC-WORKLIST-SDR-01 and AC-WORKLIST-SDR-07 (frontend/e2e/ac.spec.ts),
// which measure the closed row and the first action against the phone fold.
function RowDecision({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const opener = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  // Fetched only once the reader asks. A queue of decisions would otherwise
  // fire one read per row on arrival to fill cards nobody has opened, and the
  // row above needs none of it to draw its button.
  const approval = useApproval(item.id, open);
  // A body with no `kind` is not a proposal this card can draw: the kind
  // chooses the label, the tool chip and the autonomy dot. Treated as a failed
  // read rather than rendered, because the alternative is a throw that takes
  // the whole day's page down over one malformed answer.
  const usable = approval.data?.kind ? approval.data : undefined;
  return (
    <div className="worklist-row-decision">
      <Button
        ref={opener}
        variant="primary"
        onClick={() => setOpen(true)}
        aria-haspopup="dialog"
      >
        {t("worklist.verb.decide")}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        placement="right"
        size="wide"
        returnFocusTo={() => opener.current}
      >
        <h2 id={titleId}>{t("worklist.decision.title")}</h2>
        {usable ? (
          <ApprovalRow
            approval={usable}
            extraInvalidateKeys={[worklistKey]}
            onAlreadyDecided={() => setOpen(false)}
          />
        ) : (
          // The read has not landed, or landed unusable. Said rather than left
          // blank: a drawer that opens onto nothing reads as a broken button,
          // and the reader has already committed a tap to get here.
          <p>
            {approval.isPending
              ? t("worklist.decision.loading")
              : t("worklist.decision.unavailable")}
          </p>
        )}
      </Modal>
    </div>
  );
}

// A notice's one verb: the reader has seen it, and it leaves the lane.
// Settled here rather than routed as a link — the mutation this calls is
// the notice's own read endpoint, and there is no separate surface to send
// the reader to for "I've seen this".
function NoticeAcknowledge({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const toast = useToast();
  const acknowledge = useNoticeRead([worklistKey]);
  return (
    <Button
      small
      pending={acknowledge.isPending}
      onClick={() =>
        acknowledge.mutate(id, {
          // A rejected read leaves the button idle with nothing else on
          // screen to say so — the same rendering a click that did nothing
          // would leave. Without this the notice stays in the lane and the
          // reader has no reason to try again.
          onError: () =>
            toast.show(t("worklist.verb.acknowledgeFailed"), {
              mark: false,
            }),
        })
      }
    >
      {t("worklist.verb.acknowledge")}
    </Button>
  );
}

// Finishing a task where the reader is standing.
//
// The verb ACTS, it does not navigate. `VERB_DESTINATION` routes `complete` to
// the task's own record, which is a reasonable address and the wrong promise: a
// control labelled "Done" that opens a page leaves the task open, and the reader
// believes otherwise. The mutation exists and every other surface already uses
// it, so the row completes the task rather than renaming the promise down.
function TaskComplete({
  id,
  version,
}: Readonly<{ id: string; version: number | undefined }>) {
  const t = useT();
  const toast = useToast();
  const update = useTaskUpdate([worklistKey]);
  // mutateAsync, not mutate: it answers a promise this closure owns, so the
  // rejection is still catchable after the row has gone. `mutate`'s per-call
  // callbacks hang off the component's observer and are dropped with it.
  // The UNDO is pinned on the version the completion produced, not the one the
  // row was drawn at: ticking the task moved it on, so re-sending the older
  // number would be refused as skew by the write that has just succeeded.
  const undo = (task: string, at: number | undefined) =>
    update.mutateAsync({ id: task, version: at, body: { is_done: false } });
  return (
    <Button
      small
      variant="primary"
      pending={update.isPending}
      onClick={() =>
        update.mutate(
          { id, version, body: { is_done: true } },
          {
            // Undoable from the confirmation, the way every disposition
            // beside it is. Done REMOVES the row, so a misclick otherwise
            // costs the reader the only address they had for the task —
            // they must remember what it was to find it again.
            onSuccess: (completedAt) =>
              toast.show(t("worklist.verb.completed"), {
                action: {
                  label: t("worklist.verb.completeUndo"),
                  // The toast dismisses itself the moment the action is
                  // pressed, so a failed undo leaves the task done with the
                  // only way back already off the screen.
                  // The failure is reported from the mutationFn's own catch
                  // rather than from a per-call onError, and that is the
                  // whole reason this reads the way it does: the completion
                  // REMOVES the row, so by the time the reader presses Undo
                  // the component is unmounted and React Query has dropped
                  // the observer that per-call callbacks hang off. A refused
                  // undo then showed nothing at all — the reader pressed the
                  // one control that could undo their misclick, it failed,
                  // and the screen said nothing.
                  onAct: () => {
                    undo(id, completedAt).catch(() =>
                      toast.show(t("worklist.verb.completeUndoFailed"), {
                        mark: false,
                      }),
                    );
                  },
                },
              }),
            // A rejected PATCH otherwise leaves the button idle with nothing
            // on screen to say so — the same rendering a click that did
            // nothing would leave, and the reader has no reason to try again.
            onError: () =>
              toast.show(t("worklist.verb.completeFailed"), { mark: false }),
          },
        )
      }
    >
      {t("tasks.complete")}
    </Button>
  );
}

// A brief item's three verbs, answered where the row sits — and the ONE write
// behind all three.
//
// The row named work and offered no way to do it. `brief_item` is classified
// `today` — it is seller work, on a seller's screen — and the server sends
// `act`, `set_aside` and `dismiss` with it. None of the three is in
// VERB_DESTINATION, so the queue drew a title, a deal and a Pin button, and a
// rep looking at their most important next move had to go and find another
// screen to make it.
//
// It calls the SAME mutation Brief's brief queue calls, which already
// invalidates this queue on success — one answer to "what happens to a brief
// item", not a second one written here.
//
// A HOOK rather than a component, because the three verbs no longer share a
// container: acting is the row's call to action and the other two are ways of
// declining, so they stand on opposite edges of the row's action row. They
// still have to share the write — see `working` below — and a hook held by the
// row is how one write reaches two placements.
//
// `set_aside` posts to the brief's own snooze rather than a task's: a task's
// snooze moves a due date the rep agreed to, and a brief item's hides a
// suggestion until later in the day. The contract says so out loud, and one
// word for both is how a client writes the wrong endpoint.
function useBriefAnswer(item: WorklistItem) {
  const t = useT();
  const toast = useToast();
  const mark = useBriefItemMark();
  // ALL THREE stand down together, once one has been ANSWERED — not merely
  // while a write is in flight.
  //
  // They are three answers to one row, so a rep who acts and then dismisses has
  // answered the same item twice. `isSuccess` rather than `isPending` is what
  // makes that unreachable, and the difference is what a brief item does on
  // success: a completed task LEAVES the queue and takes its button with it,
  // while an answered brief item is patched in place and the row is still on
  // screen. Between the write settling and the refetch arriving, a second press
  // is both possible and wrong.
  //
  // Narrowing this by `mark.variables?.mark` to pend one button looks more
  // precise and does not work: `variables` is not set until React has committed
  // the mutation's state, so a second press in the same tick reads `undefined`,
  // every button stays live, and two presses become two POSTs. A guard keyed on
  // knowing WHICH verb is in flight cancels the question it was asked.
  const working = mark.isPending || mark.isSuccess;
  const answer = (next: BriefMarkRequest) => {
    mark.mutate(next, {
      // A refused answer otherwise leaves the row exactly as an unpressed one,
      // and the reader has no reason to try again — the same reason
      // NoticeAcknowledge and TaskComplete both say so.
      //
      // The error the CALLBACK was handed, not `mark.error`: that field holds
      // the state React last rendered, which on the first failure is still
      // null. The reader would be told "no cause reported" while the server
      // had named a conflict, and the retry it invites hits the same 409.
      onError: (failure) =>
        toast.show(problemMessageOf(failure, t), {
          mark: false,
        }),
    });
  };
  // Each verb is drawn only where the SERVER offered it. The lane sends all
  // three today, and a client that assumed so would keep drawing three the day
  // one is withheld — posting an answer the server did not authorise, which is
  // the failure `rowAnswer` gates every other verb against.
  const offered = (action: WorklistItem["actions"][number]) =>
    item.actions.includes(action);
  return { working, answer, offered };
}

type BriefAnswer = ReturnType<typeof useBriefAnswer>;

// Doing what the overnight ranking asked for: the row's call to action.
function BriefAct({
  item,
  brief,
}: Readonly<{ item: WorklistItem; brief: BriefAnswer }>) {
  const t = useT();
  return (
    <Button
      small
      variant="primary"
      pending={brief.working}
      onClick={() => brief.answer({ itemId: item.id, mark: "act" })}
    >
      {t("brief.act")}
    </Button>
  );
}

// The two ways of declining the ranking's pick — until later today, or for
// good. Beside the row's other secondaries rather than beside Act: they are
// what a reader chooses INSTEAD of the call to action, and the distance between
// them is what says so.
function BriefSetAsides({
  item,
  brief,
}: Readonly<{ item: WorklistItem; brief: BriefAnswer }>) {
  const t = useT();
  return (
    <>
      {brief.offered("set_aside") && (
        <Button
          small
          pending={brief.working}
          onClick={() =>
            brief.answer({
              itemId: item.id,
              mark: "snooze",
              snoozedUntil: tomorrowMorning(Date.now()),
            })
          }
        >
          {t("brief.snooze")}
        </Button>
      )}
      {brief.offered("dismiss") && (
        <Button
          small
          pending={brief.working}
          onClick={() => brief.answer({ itemId: item.id, mark: "dismiss" })}
        >
          {t("brief.dismiss")}
        </Button>
      )}
    </>
  );
}

// Running a failed rule again, from the row that reported it.
//
// The answer is not simply success or failure. The server may REFUSE with a
// reason — the firing was stopped on purpose, the rule is not cleared to repeat
// itself, or the event behind it is gone — and each of those is something the
// reader needs said in words. A refusal arrives as a normal response, so it is
// read from the resolved value rather than from an error path that would have
// to guess which refusal it was.
function AutomationRetry({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const toast = useToast();
  const retry = useAutomationRetry([worklistKey]);
  return (
    <Button
      small
      pending={retry.isPending}
      onClick={() =>
        retry.mutate(id, {
          onSuccess: (result) =>
            toast.show(
              result?.retried === true
                ? t("worklist.verb.retryStarted")
                : t(refusalMessage(result?.refusal)),
              { mark: result?.retried === true },
            ),
          // A rejected retry leaves the button idle with nothing on screen to
          // say so, which renders exactly like a click that did nothing.
          onError: () =>
            toast.show(t("worklist.verb.retryFailed"), { mark: false }),
        })
      }
    >
      {t("worklist.verb.retry")}
    </Button>
  );
}

// The reason a retry was declined, in the reader's words. An unrecognised
// refusal falls back to the generic failure rather than rendering a raw enum:
// a value this build has no wording for is still a thing that did not happen.
function refusalMessage(
  refusal: string | undefined,
): Parameters<ReturnType<typeof useT>>[0] {
  switch (refusal) {
    case "not_failed":
      return "worklist.verb.retryRefusedNotFailed";
    case "repeats_its_effect":
      return "worklist.verb.retryRefusedRepeats";
    case "trigger_event_unavailable":
      return "worklist.verb.retryRefusedEventGone";
    default:
      return "worklist.verb.retryFailed";
  }
}

// How a meeting went, recorded from the row that asked.
//
// Three buttons rather than one primary and a menu: the answers are equally
// likely and equally short, and hiding two of three behind a chevron would make
// the common case a second click. None is emerald — an outcome is a record of
// what already happened, not the day's next move, and the queue's one filled
// primary belongs to the selected row's own action.
//
// The row leaves the queue on success because the lane asks only for meetings
// with no outcome. That is also why there is no undo offered here: a corrected
// outcome is a second answer to the same question, given on the meeting itself
// where the history of both is visible, rather than a toast that disappears.
function MeetingOutcome({
  id,
  version,
}: Readonly<{ id: string; version: number | undefined }>) {
  const t = useT();
  const toast = useToast();
  const record = useMeetingOutcome([worklistKey]);
  const answer = (status: "held" | "no_show" | "canceled") => () =>
    record.mutate(
      { id, version, status },
      {
        onSuccess: () => toast.show(t("worklist.verb.meetingOutcomeRecorded")),
        // A refused write leaves the row exactly as it was, which renders
        // identically to a click that did nothing.
        onError: () =>
          toast.show(t("worklist.verb.meetingOutcomeFailed"), { mark: false }),
      },
    );
  return (
    <>
      <Button small pending={record.isPending} onClick={answer("held")}>
        {t("worklist.verb.meetingHeld")}
      </Button>
      <Button small pending={record.isPending} onClick={answer("no_show")}>
        {t("worklist.verb.meetingNoShow")}
      </Button>
      <Button small pending={record.isPending} onClick={answer("canceled")}>
        {t("worklist.verb.meetingCanceled")}
      </Button>
    </>
  );
}

// The record a reply would be filed against, or nothing.
//
// Both halves must hold. The verb says the server judged this wait answerable —
// it is mail, not a channel message the mail composer would answer in the wrong
// place. The subject says WHICH record the sent message links to, and its type
// has to be one the composer can file against: the row's own vocabulary is
// wider than RELINK_KINDS, so an `activity` subject would type-check as a
// string and fail at the composer.
function replyTarget(
  item: WorklistItem,
): { type: RelinkKind; id: string } | undefined {
  if (!item.actions.includes("reply") || !item.subject) {
    return undefined;
  }
  const type = item.subject.type;
  if (!RELINK_KINDS.includes(type as RelinkKind)) {
    return undefined;
  }
  return { type: type as RelinkKind, id: item.subject.id };
}

// Answering the buyer, over the row that named the wait.
//
// Its own component so it can hold the hook that refreshes the queue. The
// composer invalidates the RECORD timelines it knows about, and the worklist is
// not one of them — nor does the queue poll — so without the callback the row
// keeps saying nobody has replied, and keeps offering to reply again, over a
// message the reader has already answered.
function WaitingReply({
  id,
  to,
}: Readonly<{ id: string; to: { type: RelinkKind; id: string } }>) {
  const queryClient = useQueryClient();
  return (
    <ChannelReplyAction
      activityId={id}
      kind="email"
      entityType={to.type}
      entityId={to.id}
      // `worklistKey`, not `[worklistKey]`. The key IS the segment array, so
      // wrapping it once more asks for a query whose first segment is itself
      // an array — which nothing in the cache is, so the invalidation matched
      // nothing and the row a rep had just answered stayed in the waiting
      // lane until they reloaded the page.
      onSent={() =>
        void queryClient.invalidateQueries({ queryKey: worklistKey })
      }
    />
  );
}

// Saying a promise was kept, from the row that keeps asking for it.
//
// One button and not two. The endpoint settles a claim as `done` or
// `dismissed`, and they are genuinely different — kept, versus never really
// promised — but only one of them is a thing a rep does on their morning queue.
// Dismissing an extraction is a judgement about the extractor, made on the
// person's own card beside the words it was read from, where the reader can see
// what it got wrong.
function PromiseKept({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const toast = useToast();
  const settle = useClaimSettle([worklistKey]);
  return (
    <Button
      small
      variant="primary"
      pending={settle.isPending}
      onClick={() =>
        settle.mutate(
          { id, outcome: "done" },
          {
            onSuccess: () => toast.show(t("worklist.verb.promiseSettled")),
            // A refused settle leaves the row exactly as it was, which reads
            // the same as a click that did nothing.
            onError: () =>
              toast.show(t("worklist.verb.promiseSettleFailed"), {
                mark: false,
              }),
          },
        )
      }
    >
      {t("worklist.verb.promiseKept")}
    </Button>
  );
}
