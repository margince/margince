// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useRef, useState } from "react";
import { MeetingOutcome } from "../../components/meetingoutcome";
import {
  useAutomationRetry,
  useClaimSettle,
  useTaskUpdate,
} from "../../components/taskactions";
import { useCompleteTask } from "../../components/taskcomplete";
import { completeFailureKey } from "../../components/taskwritefollowup";
import { Button } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { ApprovalDecisionDrawer } from "../automations/approvaldrawer";
import { hasMoveControl, MoveButton } from "../records/movebutton";
import {
  FailedApprovalAnswer,
  NoticeAcknowledge,
} from "./worklist.acknowledge";
import {
  BriefAct,
  type BriefAnswer,
  BriefSetAsides,
} from "./worklist.briefverbs";
import { DomainQuestionAnswer } from "./worklist.domainquestion";
import { PlanWorkActions } from "./worklist.plan";
import {
  useNudgeDismissal,
  type WorklistItem,
  worklistKey,
} from "./worklist.queries";
import { replyTarget, WaitingReply } from "./worklist.reply";
import { answerBelow } from "./worklist.suggestion";

/**
 * The answer a row can carry inside it, and where in the row it goes.
 *
 * Three kinds, here and not behind a link, because the server already sent all
 * the decision needs. Sending the reader to another screen is the hand-off this
 * queue exists to remove. An approval carries its staged payload, a duplicate
 * carries both records, a notice needs only to be seen.
 *
 * Kept apart from the row, whose own job (rank, title, reasons, verbs) is at the
 * complexity the linter allows. A fourth kind extends this list, not that row.
 */

/**
 * The three places an answer can stand, and each lane picks one.
 *
 * A placement, not a node, because "the answer" is not one shape. Most lanes
 * have one call to action, and it ends the row's verbs. Some offer verbs of
 * equal weight, where a call to action would name one of three meeting outcomes
 * as the expected one. One carries a payload, not a verb.
 *
 * All three are optional. A row the server named no verb on returns none: it is
 * still real work, with nothing to press.
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
// them and the verb the server sent. A table, not a branch each: they differ
// only in which control to draw. As code, `rowAnswer` would grow one arm per
// source past the complexity ceiling.
//
// Both halves of the key matter. The source decides the control. The verb is
// the server's judgement that this row may use it: a blocked automation carries
// no `retry`, so it draws nothing here.
//
// `draw` takes the row, not its id. A task's completion is pinned to the version
// the reader decided on, and every other verb here writes too. So an entry that
// needs a second field stays in the table instead of becoming a branch.
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
  failed_approval: {
    verb: "acknowledge",
    draw: (item) => ({ primary: <FailedApprovalAnswer item={item} /> }),
  },
  automation_run: {
    verb: "retry",
    draw: (item) => ({ primary: <AutomationRetry id={item.id} /> }),
  },
  // Keyed on `keep`, the server's signal that this row may be answered at all.
  // The control then asks the row again for each button, so a seat offered one
  // of the two draws only that one. Like meeting_outcome: one entry per source.
  domain_question: {
    verb: "keep",
    draw: (item) => ({ equals: <DomainQuestionAnswer item={item} /> }),
  },
  // Two verbs of equal weight, so the row has no primary. Cancelling and saying
  // what came of it are equally likely about a meeting the product knows nothing
  // about. Leading with either would read as an expectation it has not got.
  meeting_outcome: {
    verb: "decide",
    draw: (item) => ({
      equals: (
        <MeetingOutcome
          id={item.id}
          version={item.version}
          // The row's own title field, not `itemTitle`, which takes a
          // translator and a locale to name rows with no title. This lane
          // always has one: the meeting's subject, written by whoever booked
          // it. The dialog falls back to its own heading when a row has none.
          title={item.title}
          invalidateKeys={[worklistKey]}
          cancellable={item.actions.includes("cancel")}
        />
      ),
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
  // The row's id is the contact's here, which is what the dismissal endpoint
  // takes, so this verb is offered on this lane alone. `dismiss` on brief_item
  // means something else and posts elsewhere, so this table is keyed by source.
  relationship_decay: {
    verb: "dismiss",
    draw: (item) => ({ primary: <NudgeDismiss contactId={item.id} /> }),
  },
};

export function rowAnswer(
  item: WorklistItem,
  brief: BriefAnswer,
): RowPlacement {
  if (item.source === "weekly_commitment")
    return { primary: <PlanWorkActions item={item} /> };
  if (decidable(item)) {
    return { primary: <RowDecision item={item} /> };
  }
  const below = answerBelow(item);
  if (below) {
    return below;
  }
  // Never a batch: a group row stands for a pile and names no single record.
  // Every id-keyed answer below would act on the wrong one.
  const keyed = ANSWER_BY_SOURCE[item.source];
  if (keyed && !item.batch && item.actions.includes(keyed.verb)) {
    return keyed.draw(item);
  }
  // Its own branch: this answer needs more than the row's id. The composer
  // files the sent message against a record, so the subject travels with it.
  const replyTo = replyTarget(item);
  if (replyTo) {
    return { primary: <WaitingReply item={item} to={replyTo} /> };
  }
  const primary =
    item.move?.action === "create_task" && hasMoveControl(item.move) ? (
      <MoveButton
        dealId={item.subject?.type === "deal" ? item.subject.id : undefined}
        move={item.move}
      />
    ) : undefined;
  // A suggestion's acknowledgment never replaces the supported deal move.
  if (item.source === "brief_item" && !item.batch) {
    return {
      primary,
      equals: (
        <>
          {brief.offered("act") && <BriefAct item={item} brief={brief} />}
          <BriefSetAsides item={item} brief={brief} />
        </>
      ),
    };
  }
  return { primary };
}

// Setting a lapsed contact aside for a month.
//
// Nobody is waiting on a quiet contact, so its row returns each morning.
// Without a way to say "not this one, not now", a rep who decided meets it again.
//
// Undoable from the confirmation, like every disposition beside it. The row
// leaves the lane on success. Without undo, a misclick costs the reader the only
// address they had for a contact they were not done with.
function NudgeDismiss({ contactId }: Readonly<{ contactId: string }>) {
  const t = useT();
  const toast = useToast();
  const { dismiss, restore } = useNudgeDismissal();
  return (
    <Button
      pending={dismiss.isPending}
      onClick={() =>
        dismiss.mutate(
          { contactId },
          {
            onSuccess: () =>
              toast.show(t("worklist.verb.dismissed"), {
                action: {
                  kind: "undo",
                  label: t("worklist.verb.dismissUndo"),
                  // The toast dismisses itself when the action is pressed.
                  // A failed undo leaves the contact set aside, with the only
                  // way back already off the screen.
                  //
                  // mutateAsync and a catch: the dismissal removes the row.
                  // By the time the reader presses Undo, this component is
                  // unmounted, and React Query has dropped the observer that
                  // per-call callbacks hang off. A refused undo would fail
                  // with the screen silent.
                  onAct: () => {
                    restore.mutateAsync({ contactId }).catch(() =>
                      toast.show(t("worklist.verb.dismissUndoFailed"), {
                        tone: "danger",
                      }),
                    );
                  },
                },
              }),
            onError: () =>
              toast.show(t("worklist.verb.dismissFailed"), { tone: "danger" }),
          },
        )
      }
    >
      {t("worklist.verb.dismiss")}
    </Button>
  );
}

// Whether this row is a decision a reader answers here.
//
// The queue holds no authority of its own: the card is the one the record page
// draws, posting to the same endpoint. The queue adds that the decision is
// answerable where it was ranked, not on a second screen.
function decidable(item: WorklistItem): boolean {
  return item.actions.includes("decide") && item.source === "approval";
}

// The decision itself, fetched whole because a row cannot carry it, and
// answered in a drawer, not in the row.
//
// Inline, the card makes the queue unusable on a phone. It carries evidence, a
// draft and three answers. At 390x844 it stands 440px tall in a row capped at
// 208, and the page's first primary action lands 920px down an 844px screen.
//
// The row keeps the decision's summary and one button. The drawer holds the
// card: the ApprovalRow the record page draws, posting to the same endpoint.
// So the queue adds no authority; the decision is answerable where it ranks.
//
// Held by: AC-WORKLIST-SDR-01 and AC-WORKLIST-SDR-07 (frontend/e2e/ac.spec.ts),
// which measure the closed row and the first action against the phone fold.
function RowDecision({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const opener = useRef<HTMLButtonElement>(null);
  return (
    // A test id, not a class: which rows offer a decision is what a
    // screen journey counts, and nothing draws this wrapper.
    <div data-testid="worklist-row-decision">
      <Button
        ref={opener}
        variant="primary"
        onClick={() => setOpen(true)}
        aria-haspopup="dialog"
      >
        {t("worklist.verb.decide")}
      </Button>
      <ApprovalDecisionDrawer
        approvalId={item.id}
        open={open}
        onClose={() => setOpen(false)}
        returnFocusTo={() => opener.current}
      />
    </div>
  );
}

// Finishing a task where the reader is standing.
//
// The verb acts; it does not navigate. `VERB_DESTINATION` routes `complete` to
// the task's own record, a fair address and the wrong promise. A "Done" control
// that opens a page leaves the task open while the reader believes otherwise.
// Every other surface uses the mutation, so the row completes the task too.
function TaskComplete({
  id,
  version,
}: Readonly<{ id: string; version: number | undefined }>) {
  const t = useT();
  const toast = useToast();
  const update = useTaskUpdate([worklistKey]);
  // A rejected PATCH otherwise leaves the button idle with nothing on screen
  // to say so, and the reader has no reason to try again.
  const complete = useCompleteTask(update, (error) =>
    toast.show(t(completeFailureKey(error)), { tone: "danger" }),
  );
  return (
    <Button
      variant="primary"
      pending={update.isPending}
      onClick={() => complete(id, version)}
    >
      {t("tasks.complete")}
    </Button>
  );
}

// Running a failed rule again, from the row that reported it.
//
// The answer is more than success or failure. The server may refuse with a
// reason: the firing was stopped, the rule is not cleared to repeat, or the
// event behind it is gone. The reader needs each said in words. A refusal is a
// normal response, read from the resolved value, not guessed at on an error path.
function AutomationRetry({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const toast = useToast();
  const retry = useAutomationRetry([worklistKey]);
  return (
    <Button
      pending={retry.isPending}
      onClick={() =>
        retry.mutate(id, {
          onSuccess: (result) =>
            toast.show(
              result?.retried === true
                ? t("worklist.verb.retryStarted")
                : t(refusalMessage(result?.refusal)),
              // `info` and not `success`: an accepted retry has started, not
              // finished, and the work is still in flight.
              { tone: result?.retried === true ? "info" : "danger" },
            ),
          // A rejected retry leaves the button idle with nothing on screen to
          // say so, which looks like a click that did nothing.
          onError: () =>
            toast.show(t("worklist.verb.retryFailed"), { tone: "danger" }),
        })
      }
    >
      {t("worklist.verb.retry")}
    </Button>
  );
}

// The reason a retry was declined, in the reader's words. An unknown refusal
// falls back to the generic failure, not a raw enum. A value this build cannot
// word is still a thing that did not happen.
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

// How a meeting went is recorded by `MeetingOutcome` in
// components/meetingoutcome.tsx. "Cancelled" writes that one word from the card;
// "Update" opens the composer bound to this meeting, where the outcome is typed.
// Its dialog reads and patches the meeting: a form, a query and a mutation. That
// is a screen's worth of code, not one row verb, so it has its own file.

// Saying a promise was kept, from the row that keeps asking for it.
//
// One button, not two. The endpoint settles a claim as `done` (kept) or
// `dismissed` (never promised). Only the first is a thing a rep does on the
// morning queue. Dismissing an extraction is a judgement about the extractor.
// It is made on the contact's own card, beside the words it was read from, where
// the reader can see what it got wrong.
function PromiseKept({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const toast = useToast();
  const settle = useClaimSettle([worklistKey]);
  return (
    <Button
      variant="primary"
      pending={settle.isPending}
      onClick={() =>
        settle.mutate(
          { id, outcome: "done" },
          {
            onSuccess: () => toast.show(t("worklist.verb.promiseSettled")),
            // A refused settle leaves the row as it was, which reads
            // the same as a click that did nothing.
            onError: () =>
              toast.show(t("worklist.verb.promiseSettleFailed"), {
                tone: "danger",
              }),
          },
        )
      }
    >
      {t("worklist.verb.promiseKept")}
    </Button>
  );
}
