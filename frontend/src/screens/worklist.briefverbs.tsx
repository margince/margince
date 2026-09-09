// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Answering a brief item where it is ranked: the row's three verbs and the one
// write behind them.
//
// Split from worklist.row.tsx at the file-length ceiling, on the seam the row
// already had — these three components and their hook are the only place a
// worklist row speaks to the BRIEF's endpoints rather than to an activity's,
// and that distinction is the whole reason they are written carefully.

import { Button } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { BriefMarkRequest } from "./brief.queries";
import { useBriefItemMark } from "./brief.queries";
import { tomorrowMorning } from "./briefqueue";
import { problemMessageOf } from "./common";
import type { WorklistItem } from "./worklist.queries";

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
export function useBriefAnswer(item: WorklistItem) {
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
  const answer = (next: BriefMarkRequest, done?: () => void) => {
    mark.mutate(next, {
      onSuccess: () => {
        // A TAKE-BACK UN-ANSWERS THE ITEM, so the latch above has to let go.
        //
        // `isSuccess` is what stops a rep answering one row twice, and it stays
        // true for the life of the mutation. The unsnooze shares that mutation,
        // so without this reset the item comes back to the queue with all three
        // verbs inert: the row offers "set aside" and the click does nothing,
        // until something else remounts it.
        //
        // Reset only on the way back. Every other verb leaves the item answered
        // and the latch is doing its job.
        if (next.mark === "unsnooze") {
          // AFTER the mutation finishes settling, not inside the callback:
          // TanStack writes its success state when onSuccess returns, so a
          // reset from within it is immediately overwritten and the latch
          // stays shut.
          queueMicrotask(() => mark.reset());
        }
        done?.();
      },
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

export type BriefAnswer = ReturnType<typeof useBriefAnswer>;

// Doing what the overnight ranking asked for: the row's call to action.
export function BriefAct({
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
export function BriefSetAsides({
  item,
  brief,
}: Readonly<{ item: WorklistItem; brief: BriefAnswer }>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  // SET ASIDE UNTIL WHEN, and a way back from it.
  //
  // The snooze said nothing and could not be taken back: a rep who set aside
  // the wrong row waited for the condition to lift. The toast names the moment
  // the item returns, in the reader's own zone — "until tomorrow" is the
  // button's promise and the hour is the fact behind it — and carries the undo
  // that was missing, which posts the take-back the server now serves.
  const setAside = () => {
    const until = tomorrowMorning(Date.now());
    brief.answer({ itemId: item.id, mark: "snooze", snoozedUntil: until }, () =>
      toast.show(
        t("brief.snooze.done", {
          at: formatDateTime(until, locale, viewerZone()),
        }),
        {
          action: {
            label: t("brief.snooze.undo"),
            // A failed take-back is reported by `answer` itself, which raises
            // the server's own message as a second toast — better than a fixed
            // string here, and why this needs no error branch of its own. It
            // matters because the toast dismisses the moment the action is
            // pressed: without a word the item stays hidden and the only way
            // back has just left the screen.
            //
            // KNOWN LIMIT, shared with the disposition undo next door: the
            // callback belongs to this row's mutation, so a rep who scrolls the
            // row out of the list before pressing Undo gets no message if the
            // write then fails. The take-back is refused rather than lost — the
            // item stays set aside and returns on its own condition — but they
            // are not told. Closing it means raising the failure from the toast
            // itself, which both undos would want and neither has.
            onAct: () => brief.answer({ itemId: item.id, mark: "unsnooze" }),
          },
        },
      ),
    );
  };
  return (
    <>
      {brief.offered("set_aside") && (
        <Button small pending={brief.working} onClick={setAside}>
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
