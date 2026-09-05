// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One row of the day, and the verbs it offers.
//
// Split from the screen because they answer different questions. The screen
// decides WHAT the page shows — whose day, which cut, which headings. A row
// decides how one piece of work reads and where each of its verbs goes, and
// that is the half a reader of either question does not need the other for.

import { useId, useRef, useState } from "react";
import { Badge, Button, Modal } from "../design-system/atoms";
import { PanelRow } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { translatePlural, useLocale, useT } from "../i18n";
import { ApprovalRow } from "./approvalrow";
import { tomorrowMorning } from "./briefqueue";
import { problemMessageOf } from "./common";
import { type BriefMarkRequest, useBriefItemMark } from "./home.queries";
import { useNoticeRead, useTaskUpdate } from "./taskactions";
import {
  comparisonText,
  consequenceText,
  dealFactsText,
  isUnprepared,
  itemTitle,
  moveHref,
  moveLabel,
  phrasedReasons,
  reasonText,
  rowHref,
  whenText,
} from "./worklist.copy";
import { DispositionVerbs, PutDownByThumb } from "./worklist.dispositions";
import { WaitingEmailLine } from "./worklist.emailtitle";
import { ReassignControl } from "./worklist.manager";
import { PairDecision } from "./worklist.pair";
import {
  useApproval,
  useNudgeDismissal,
  usePinRow,
  type WorklistItem,
  worklistKey,
} from "./worklist.queries";
import { syncHealthDetail } from "./worklist.synchealth";
import { VerdictLine } from "./worklist.verdict";

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
  const href = rowHref(item);
  const title = itemTitle(item, t, locale);
  const facts = dealFactsText(item, t, locale, zone);
  const sample = namedMembers(item);
  // The clock this row is racing. A meeting said "starting shortly" whether it
  // began in four minutes or in fifty, and a task said "Overdue" without saying
  // by how long — on the two rows whose whole claim is a moment.
  const when = whenText(item, t, locale, zone, new Date());
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
        <div className="worklist-row-text">
          {/* A waiting EMAIL names itself with the canonical row — the same one
            the timeline draws — so the queue shows the message rather than a
            sentence about it. Everything else keeps the title line it had, and
            the badges below stay on both: they say where the row sits in the
            day, which the email row does not answer. */}
          {emailOpener && <WaitingEmailLine item={item} onOpen={emailOpener} />}
          <p className="t-body worklist-row-title">
            {emailOpener ? null : href ? (
              <a className="entity-link" href={href}>
                {title}
              </a>
            ) : (
              title
            )}
            <Badge>{t(`worklist.category.${item.category}` as const)}</Badge>
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
        {item.batch && onReview ? (
          <BatchVerb onReview={onReview} />
        ) : (
          <RowVerbs item={item} href={href} move={moveHref(item)} />
        )}
        {/* The ways this row can be PUT DOWN, as the server declares them. Drawn
          from `dispositions` rather than inferred from `source`: which rows a
          rep may judge is a server rule, and a client keeping its own copy
          draws a verb that 404s or hides one the rep is entitled to. */}
        <DispositionVerbs item={item} />
        {/* The reader's own override, on every row that can carry one. It is not
          a disposition — those put a row DOWN, and this lifts one up — so it is
          drawn beside them rather than among them. */}
        <PinVerb item={item} />
        {/* Only a task carries an assignee, so only a task can be handed on. A
          group row stands for a pile and names no single activity to move.

          Offered on the reader's OWN queue too: handing work on is not a
          manager's verb, and gating it on a selected rep left somebody
          looking at their own list with no way to pass a task along. Who is
          excluded from the destinations follows the queue rather than this
          condition — ReassignControl falls back to the reader when no rep is
          selected, so the current holder is never offered as the new one. */}
        {item.source === "task" && !item.batch && (
          <ReassignControl item={item} owner={owner} />
        )}
        <RowAnswer item={item} />
      </PutDownByThumb>
    </PanelRow>
  );
}

/**
 * The answer a row can carry INSIDE it.
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
function RowAnswer({ item }: Readonly<{ item: WorklistItem }>) {
  if (decidable(item)) {
    return <RowDecision item={item} />;
  }
  if (item.source === "dedupe_candidate" && item.pair) {
    return <PairDecision item={item} />;
  }
  if (item.source === "notice" && item.actions.includes("acknowledge")) {
    return <NoticeAcknowledge id={item.id} />;
  }
  // A task the server says can be finished, finished HERE. Not a batch: a group
  // row stands for a pile and names no single activity to complete.
  if (
    item.source === "task" &&
    !item.batch &&
    item.actions.includes("complete")
  ) {
    return <TaskComplete id={item.id} />;
  }
  // A quiet contact the reader has decided not to chase. The row's id IS the
  // person's, which is what the dismissal endpoint takes — the pairing is why
  // this verb is offered on this lane and nowhere else.
  if (
    item.source === "relationship_decay" &&
    !item.batch &&
    item.actions.includes("dismiss")
  ) {
    return <NudgeDismiss personId={item.id} />;
  }
  // A brief item's three verbs. Source-checked rather than verb-checked:
  // `dismiss` also belongs to relationship_decay above, where it means
  // something else entirely and posts somewhere else.
  if (item.source === "brief_item" && !item.batch) {
    return <BriefVerbs item={item} />;
  }
  return null;
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
    <div className="worklist-row-verbs">
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
    </div>
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
 */
function RowReasons({ reasons }: Readonly<{ reasons: readonly string[] }>) {
  const { locale } = useLocale();
  if (reasons.length === 0) {
    return null;
  }
  const said = reasons.slice(0, REASONS_BEFORE_THE_FOLD);
  const folded = reasons.slice(REASONS_BEFORE_THE_FOLD);
  if (folded.length === 0) {
    return <p className="t-caption worklist-row-because">{said.join(" · ")}</p>;
  }
  return (
    <details className="worklist-row-because-fold">
      <summary className="t-caption worklist-row-because">
        {said.join(" · ")}{" "}
        <span className="worklist-row-because-more">
          {translatePlural(locale, "worklist.because.more", folded.length, {
            // The reader's own notation, not String(): a count drawn for a
            // person goes through the formatter like every other magnitude,
            // and jsx-magnitude.test.ts holds that for the whole tree.
            count: formatNumber(folded.length, locale),
          })}
        </span>
      </summary>
      <p className="t-caption worklist-row-because">{folded.join(" · ")}</p>
    </details>
  );
}

/**
 * Everything the row says about itself under the title, in the order a reader
 * needs it.
 *
 * When it happens, what it is worth, why it is ranked where it is, what doing
 * nothing costs, and why it beat the row below. Each is absent when the server
 * sent nothing for it — a caption drawn empty is a line of furniture the reader
 * has to look past on every row.
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
      {/* When it starts, or when it is due. Above the reasons because it is the
          fact those reasons are ABOUT: "starting shortly" explains a rank, and
          this says what time. */}
      {/* WHEN, WHAT IT IS WORTH and WHY, on one wrapping line rather than
          three stacked ones. Each is a fragment — "due 15:00", "€40k", "due
          today · nobody owns it" — and three fragments of a dozen characters
          each took three full lines of a 390px row, which is 38px of a 176px
          ceiling spent on whitespace beside three short phrases. They stay
          separate elements, so a reader still meets them in the same order and
          a screen reader still reads three facts; only the line breaks between
          them go. Above that width they stack as before, because a wide row has
          the height and stacked lines are easier to scan. */}
      <div className="worklist-row-facts-line">
        {when && <p className="t-caption worklist-row-when">{when}</p>}
        {facts && <p className="t-caption worklist-row-facts">{facts}</p>}
        <RowReasons reasons={reasons} />
      </div>
      {/* What it costs to do nothing. The question a queue exists to answer,
          and the one the lane feed had no field for. */}
      {consequence && (
        <p className="t-caption worklist-row-consequence">{consequence}</p>
      )}
      {/* Why this row beat the one below it. Absent on the last row, which has
          nothing below it to beat. */}
      {above && <p className="t-caption worklist-row-above">{above}</p>}
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
    <div className="worklist-row-verbs">
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
    </div>
  );
}

// Finishing a task where the reader is standing.
//
// The verb ACTS, it does not navigate. `VERB_DESTINATION` routes `complete` to
// the task's own record, which is a reasonable address and the wrong promise: a
// control labelled "Done" that opens a page leaves the task open, and the reader
// believes otherwise. The mutation exists and every other surface already uses
// it, so the row completes the task rather than renaming the promise down.
function TaskComplete({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const toast = useToast();
  const update = useTaskUpdate([worklistKey]);
  // mutateAsync, not mutate: it answers a promise this closure owns, so the
  // rejection is still catchable after the row has gone. `mutate`'s per-call
  // callbacks hang off the component's observer and are dropped with it.
  const undo = (task: string) =>
    update.mutateAsync({ id: task, body: { is_done: false } });
  return (
    <div className="worklist-row-verbs">
      <Button
        small
        variant="primary"
        pending={update.isPending}
        onClick={() =>
          update.mutate(
            { id, body: { is_done: true } },
            {
              // Undoable from the confirmation, the way every disposition
              // beside it is. Done REMOVES the row, so a misclick otherwise
              // costs the reader the only address they had for the task —
              // they must remember what it was to find it again.
              onSuccess: () =>
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
                      undo(id).catch(() =>
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
    </div>
  );
}

// A brief item's three verbs, answered where the row sits.
//
// The row named work and offered no way to do it. `brief_item` is classified
// `today` — it is seller work, on a seller's screen — and the server sends
// `act`, `set_aside` and `dismiss` with it. None of the three is in
// VERB_DESTINATION, so the queue drew a title, a deal and a Pin button, and a
// rep looking at their most important next move had to go and find another
// screen to make it.
//
// It calls the SAME mutation Home's brief queue calls, which already
// invalidates this queue on success — one answer to "what happens to a brief
// item", not a second one written here.
//
// `set_aside` posts to the brief's own snooze rather than a task's: a task's
// snooze moves a due date the rep agreed to, and a brief item's hides a
// suggestion until later in the day. The contract says so out loud, and one
// word for both is how a client writes the wrong endpoint.
function BriefVerbs({ item }: Readonly<{ item: WorklistItem }>) {
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
  // the failure `RowAnswer` gates every other verb against.
  const offered = (action: WorklistItem["actions"][number]) =>
    item.actions.includes(action);
  return (
    <div className="worklist-row-verbs">
      {offered("act") && (
        <Button
          small
          variant="primary"
          pending={working}
          onClick={() => answer({ itemId: item.id, mark: "act" })}
        >
          {t("home.act")}
        </Button>
      )}
      {offered("set_aside") && (
        <Button
          small
          pending={working}
          onClick={() =>
            answer({
              itemId: item.id,
              mark: "snooze",
              snoozedUntil: tomorrowMorning(Date.now()),
            })
          }
        >
          {t("home.snooze")}
        </Button>
      )}
      {offered("dismiss") && (
        <Button
          small
          pending={working}
          onClick={() => answer({ itemId: item.id, mark: "dismiss" })}
        >
          {t("home.dismiss")}
        </Button>
      )}
    </div>
  );
}

// The way into a group.
//
// It narrows the queue to decisions rather than opening a screen of its own:
// that screen is its own piece of work, and a row whose only verb led nowhere
// would be worse than the pile it replaced.
//
// A button, not a link. The dials live in this screen's state today, so an
// address carrying `?filter=decisions` would be read by nobody and the control
// would do nothing — which is the defect it exists to avoid. Moving them into
// the URL is the right shape and is its own change.
function BatchVerb({ onReview }: Readonly<{ onReview: () => void }>) {
  const t = useT();
  return (
    <div className="worklist-row-verbs">
      <Button small onClick={onReview}>
        {t("worklist.verb.review_batch")}
      </Button>
    </div>
  );
}

// What this row offers, as the item itself declares it.
//
// Every verb is a LINK to the surface that owns it rather than a mutation from
// here: this queue adds no authority of its own, so deciding an approval goes
// to the decision surface and merging a pair to the dedupe queue, exactly as
// they do from any other door. Rendering a button that acted here would be a
// second place for those rules to live.
//
// A verb whose destination this page cannot name draws nothing. A control that
// looks pressable and goes nowhere is worse than no control.
function RowVerbs({
  item,
  href,
  move,
}: Readonly<{
  item: WorklistItem;
  href: string | undefined;
  move: string | undefined;
}>) {
  const t = useT();
  const drawn = new Set<string>();
  type Verb = {
    action: WorklistItem["actions"][number];
    destination: string;
  };
  const verbs = item.actions.flatMap<Verb>((action) => {
    if (action === "decide") {
      const to = decideDestination(item, href);
      return to ? [{ action, destination: to }] : [];
    }
    const route = VERB_DESTINATION[action];
    if (!route) {
      // A verb this build cannot route draws nothing. A control that looks
      // pressable and goes nowhere is worse than no control.
      return [];
    }
    const destination = route(href);
    if (!destination) {
      return [];
    }
    // One control per DESTINATION. `complete` and `snooze` both open the
    // record this row is about, and two identical "Open" links side by side
    // ask the reader to choose between the same thing twice.
    const key = `${VERB_LABEL[action](t)}|${destination}`;
    if (drawn.has(key)) {
      return [];
    }
    drawn.add(key);
    return [{ action, destination }];
  });
  if (verbs.length === 0 && !move) {
    return null;
  }
  return (
    <div className="worklist-row-verbs">
      {/* The step the product already worked out, offered where the reader is
          standing rather than on a screen they have to go and find. */}
      {move && (
        <a className="link-button" href={move}>
          {/* THE LABEL MOVES WITH THE ROUTE AND WITH THE VERB. Where the
              address opens the composer the label is the act; where it only
              reaches the record it says so. And it names the verb the SERVER
              chose, so an opening outreach is not offered as a reply to a
              conversation nobody has had. */}
          {moveLabel(item, t)}
        </a>
      )}
      {verbs.map(({ action, destination }) => (
        <a key={action} className="link-button" href={destination}>
          {VERB_LABEL[action](t)}
        </a>
      ))}
    </div>
  );
}

// Where each verb lives. A total map over the ones this page can route, so a
// verb the contract adds either gets a destination here or is not drawn —
// never a button that does nothing.
const VERB_DESTINATION: Partial<
  Record<
    WorklistItem["actions"][number],
    (href: string | undefined) => string | undefined
  >
> = {
  // `decide` and `merge` are deliberately absent: the surface that answers
  // them IS this page, so a link would send the reader where they already are.
  // They come back when the decision card is drawn inline, which is its own
  // piece of work.
  //
  // `acknowledge` is absent too — see NoticeAcknowledge, which draws it
  // inline instead of through this table.
  //
  // Everything routable is the record the row is about.
  open: (href) => href,
  complete: (href) => href,
  snooze: (href) => href,
};

// The one verb whose routing depends on the SOURCE rather than only the verb.
//
// `decide` is answered inline for an approval — the card is right there, so a
// link would send the reader where they already are. An introduction ask has no
// inline card: its four answers are the colleague's own, given on the contact's
// Network tab. Without this the ask row names somebody waiting and offers
// nothing at all, which is the worst of both.
function decideDestination(
  item: WorklistItem,
  href: string | undefined,
): string | undefined {
  return item.source === "introduction_request" ? href : undefined;
}

// What each routable verb is called. Spelled as a map of functions rather than
// a composed key, so a verb the contract adds without copy here does not
// compile — which is the only way this cannot reach a reader as a raw word.
const VERB_LABEL: Record<
  WorklistItem["actions"][number],
  (t: ReturnType<typeof useT>) => string
> = {
  decide: (t) => t("worklist.verb.decide"),
  merge: (t) => t("worklist.verb.merge"),
  open: (t) => t("worklist.verb.open"),
  complete: (t) => t("worklist.verb.complete"),
  snooze: (t) => t("worklist.verb.snooze"),
  acknowledge: (t) => t("worklist.verb.acknowledge"),
  // The briefing queue's three verbs. Named here because the map is total over
  // the contract's actions — they route nowhere from this page yet, so
  // VERB_DESTINATION does not carry them and no control is drawn.
  act: (t) => t("worklist.verb.open"),
  dismiss: (t) => t("worklist.verb.open"),
  set_aside: (t) => t("worklist.verb.open"),
};

// The day's figures, and the dials that narrow them.

// The reader's own override: this row leads their day, whatever the ranking
// chose.
//
// The ranking has carried a pin level since it was written and, until the store
// shipped, nothing could set it. Every other control on this page changes what
// the SERVER thinks — a disposition, a filter, a scope. This is the only one
// that says "I know, and I want this first anyway", which is the difference
// between a queue a rep works and a queue a rep argues with.
//
// WHAT IT READS to know which way to toggle: the row's own `pinned` reason. The
// server states it on a pinned row, so the client asks the response rather than
// keeping a second record of what it pressed — a local flag would disagree with
// the page the moment the reader pinned from another tab, and the button would
// offer to pin a row that already leads their day.
//
// A BATCH row is skipped. Its id is synthetic and minted by the fold, so a pin
// on one names a group that will not exist under that key on the next read.
function PinVerb({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const toast = useToast();
  const pin = usePinRow();
  if (item.batch) {
    return null;
  }
  const pinned = (item.because ?? []).some((why) => why.kind === "pinned");
  return (
    <div className="worklist-row-verbs">
      <Button
        small
        pending={pin.isPending}
        onClick={() =>
          pin.mutate(
            { source: item.source, rowId: item.id, pinned },
            {
              // A refused write otherwise leaves the button exactly as an
              // unpressed one looks, and the row keeps the place it had — so
              // the reader is told nothing and sees nothing change.
              onError: () =>
                toast.show(
                  t(
                    pinned
                      ? "worklist.verb.unpinFailed"
                      : "worklist.verb.pinFailed",
                  ),
                  { mark: false },
                ),
            },
          )
        }
      >
        {t(pinned ? "worklist.verb.unpin" : "worklist.verb.pin")}
      </Button>
    </div>
  );
}
