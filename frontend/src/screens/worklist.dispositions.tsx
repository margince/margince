// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Putting a waiting message down.
//
// The queue's claim is that it is finite: work it and it empties. A message the
// rep has already judged — the newsletter that is not a customer, the reply
// that belongs to a colleague, the one they will get to on Thursday — had no
// way to leave, so it came back every morning and the count above it stayed
// wrong.
//
// Every verb here is undoable from the confirmation it raises, because each one
// REMOVES something from the reader's view. A control whose effect is a
// disappearance needs its way back offered at the moment of the disappearance.
// Finding it again afterwards means knowing which judgement you made.

import { createContext, useContext, useRef, useState } from "react";
import { Button } from "../design-system/atoms";
import { SwipeRow } from "../design-system/swiperow";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { type Locale, translatePlural, useLocale, useT } from "../i18n";
import { useFoldedViewport } from "../lib/viewport";
import { useMarkRowsDone } from "./worklist.donequeries";
import {
  type ReopenCondition,
  useClearDisposition,
  useSetDisposition,
  type WorklistDisposition,
  type WorklistItem,
} from "./worklist.queries";
import {
  DispositionButton,
  knownDispositions,
  type PutDownAnswer,
  SWIPE_SIDES,
  writeState,
} from "./worklist.verbbutton";

export { SWIPE_SIDES };

// How long a snooze lasts, when the reader does not say.
//
// One working day, because the verb answers "not now, but today is not over" —
// a rep reaching for it at ten in the morning means tomorrow morning, not this
// afternoon. It stays the default the plain press takes, so the fast path is
// still one click.
export const SNOOZE_DAYS = 1;

// The spans a reader can choose instead.
//
// The server takes any future instant — `snoozed_until` is caller-supplied and
// validated only as "later than now" — so tomorrow was never the product's
// limit, it was this file's. A rep who knows a customer is away all week had to
// press the same button seven mornings running.
//
// Days rather than named moments ("this evening", "Monday"), because a named
// moment is a claim about the reader's calendar: "Monday" from a Friday is
// three days and from a Monday is seven, and a queue that guessed wrong would
// hide work for four days nobody asked for. A span says exactly what it does.
export const SNOOZE_SPANS = [1, 3, 7] as const;

export type SnoozeSpan = (typeof SNOOZE_SPANS)[number];

// The two answers a span cannot give.
//
// Every span above is a guess about when the world will move, and a rep making
// that guess is usually wrong in one of two directions: too short and the row
// is back while the customer is still silent, too long and the customer waited
// on them. These say what they are actually waiting for, and the server decides
// when it happened.
//
// `meeting` is deliberately NOT here. It needs the rep to name WHICH meeting,
// which is a picker over the record's calendar rather than a line in this list
// — offering it without one would send a condition the server refuses. The
// server takes it today; this surface does not yet ask for it.
export const SNOOZE_EVENTS = ["reply"] as const satisfies readonly Exclude<
  ReopenCondition,
  "time" | "meeting"
>[];

export type SnoozeEvent = (typeof SNOOZE_EVENTS)[number];

// Which undo REACH each judgement takes back.
//
// `not_sales` bound the whole workspace, so its undo has to say `thread` or it
// would clear a reader-state row that was never written and leave the thread
// hidden from everybody. The two personal states clear the reader's own.
function undoScope(disposition: WorklistDisposition): "mine" | "thread" {
  return disposition === "not_sales" ? "thread" : "mine";
}

type T = ReturnType<typeof useT>;

// Putting one row down, as an action the CALLER places.
//
// The verbs and the swipe are two placements of one capability, so the write
// lives here and neither owns the other: the row draws buttons where there is
// width and hands the same `put` to the gesture where there is not. Two copies
// of this mutation is how one placement keeps working after the other stops.
export function usePutDown(item: WorklistItem) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const set = useSetDisposition();
  const clear = useClearDisposition();
  const done = useMarkRowsDone();
  const offered = knownDispositions(item.dispositions);
  // The answer the last press sent. Read only through `writing` below, which is
  // why nothing clears it: an answer that is not in flight is not consulted,
  // and every press that puts one in flight records it first.
  const [pressed, setPressed] = useState<PutDownAnswer | undefined>(undefined);
  // Whether a write from this row is out, read SYNCHRONOUSLY and at the press.
  //
  // `set.isPending` answers the same question for the DRAWING, and has to —
  // changing a ref renders nothing. It cannot guard the press, because a
  // handler belongs to the render that made it: `SwipeRow` keeps the staged
  // action in its own state, so a confirm can run a `put` whose closure was
  // built before any write started and whose `isPending` is false while a
  // request is in the air. Two contradictory judgements reached the server for
  // one message that way, and two undo toasts followed, each offering to clear
  // what the other had just written.
  const writeIsOut = useRef(false);
  const put = (
    disposition: WorklistDisposition,
    // A span or an event, never both: the two are alternative answers to "come
    // back when", and the server refuses a snooze carrying a moment it does not
    // wait for.
    until?: SnoozeSpan | SnoozeEvent,
  ) => {
    // ONE write per row at a time, held here rather than at each control: the
    // verbs refuse their own second press, and a swipe confirm does not go
    // through them.
    if (writeIsOut.current) {
      return;
    }
    const event = typeof until === "string" ? until : undefined;
    const days = typeof until === "number" ? until : undefined;
    writeIsOut.current = true;
    setPressed({ disposition, until });
    // Done goes to the Worklist's own endpoint on every row, so a row that is
    // not a message takes it too. The other judgements are a message's own.
    const handled = disposition === "handled";
    const row = { source: item.source, row_id: item.id };
    // A failed undo needs saying: the toast is gone once Undo is pressed, so
    // silence would leave the row hidden with no way back.
    const undoFailed = {
      onError: () =>
        toast.show(t("worklist.disposition.undoFailed"), { tone: "danger" }),
    };
    const settled = {
      onSuccess: () =>
        toast.show(doneText(disposition, until, item.source, t, locale), {
          action: {
            kind: "undo",
            label: t("worklist.disposition.undo"),
            onAct: () =>
              handled
                ? done.mutate({ rows: [row], reopen: true }, undoFailed)
                : clear.mutate(
                    { activityId: item.id, scope: undoScope(disposition) },
                    undoFailed,
                  ),
          },
        }),
      onError: () =>
        toast.show(t("worklist.disposition.failed"), { tone: "danger" }),
      // Settled either way, so a refusal releases the row rather than leaving
      // it unanswerable until the reader reloads.
      onSettled: () => {
        writeIsOut.current = false;
      },
    };
    if (handled) {
      done.mutate({ rows: [row] }, settled);
      return;
    }
    set.mutate(
      {
        activityId: item.id,
        disposition,
        ...(disposition === "snooze"
          ? event
            ? { reopenOn: event }
            : {
                reopenOn: "time" as const,
                snoozedUntil: snoozeUntil(days).toISOString(),
              }
          : {}),
      },
      settled,
    );
  };
  return {
    offered,
    put,
    // The answer whose write is out, or nothing while none is. Not a bare
    // boolean: every control in the band reads the same value, and what each
    // one needs to know is whether the write out is ITS OWN.
    writing: set.isPending || done.isPending ? pressed : undefined,
    t,
    locale,
  };
}

// The whole row, answerable with the thumb where there is no width for verbs.
//
// It wraps the ROW rather than the verbs, and that is the difference between a
// feature and a demo: the row's verbs are ONE item of its layout, so a gesture
// mounted around them would ask a reader to swipe an 83px chip and would draw
// an empty box on a row whose only judgements are the ones the chip does not
// carry. What this box costs the row's layout is stated in worklist.row.css.
//
// Above the fold it draws the row and nothing else. The row's own verbs carry
// the judgements there, and this adds nothing they do not already offer.
export function PutDownByThumb({
  item,
  children,
}: Readonly<{ item: WorklistItem; children: React.ReactNode }>) {
  const folded = useFoldedViewport();
  // ONE MUTATION PER ROW, held here because this is the component both
  // placements sit inside. Two `usePutDown` calls on one row are two mutations
  // with two `pending` flags, and neither disables the other: a menu press
  // followed by a swipe confirm wrote the same judgement twice and raised two
  // toasts, each offering an Undo for a state the other had already cleared.
  // Measured before this was lifted — two PUTs from one row.
  // PER SIDE, because the walk is a position in one direction's list. A single
  // counter shared by both would be moved by a flick the other way, so a reader
  // alternating directions would never step through either side's judgements.
  const [walked, setWalked] = useState({ start: 0, end: 0 });
  const write = usePutDown(item);
  const { offered, put, t } = write;
  if (!folded || offered.length === 0) {
    return <PutDown value={write}>{children}</PutDown>;
  }
  return (
    <SwipeRow
      cancelLabel={t("worklist.disposition.swipeCancel")}
      // Advanced on the STAGE rather than the confirm. The walk is how a second
      // flick reaches the second judgement a direction carries, and a counter
      // that moved only when the reader committed would never move for one who
      // dismisses — leaving that judgement unreachable on a phone.
      onStage={(side) =>
        setWalked((seen) => ({ ...seen, [side]: seen[side] + 1 }))
      }
      {...swipeActions(offered, t, put, walked)}
    >
      <PutDown value={write}>{children}</PutDown>
    </SwipeRow>
  );
}

// The row's one write, handed to whichever control the width leaves standing.
//
// A context rather than a prop, because the verbs are drawn deep inside the
// row's own tree — the caller composes them beside the title and the rank, and
// threading the write through every one of those would make the row's shape
// the write's business.
const PutDownContext = createContext<PutDown | null>(null);

function PutDown({
  value,
  children,
}: Readonly<{ value: PutDown; children: React.ReactNode }>) {
  return (
    <PutDownContext.Provider value={value}>{children}</PutDownContext.Provider>
  );
}

export type PutDown = ReturnType<typeof usePutDown>;

// The row's one write, the one PutDownByThumb holds, so a press in the menu and
// a swipe confirm cannot write twice. Verbs drawn outside a row get their own.
// Both hooks run either way, because a hook cannot be called conditionally.
export function useRowPutDown(item: WorklistItem): PutDown {
  const shared = useContext(PutDownContext);
  const own = usePutDown(item);
  return shared ?? own;
}

// Done, beside the row's answer: the one judgement that finishes the row. Below
// the fold it is a line of the row's menu instead, for the width arithmetic in
// design-system/swiperow.tsx's header.
export function DoneVerb({ write }: Readonly<{ write: PutDown }>) {
  const folded = useFoldedViewport();
  const { offered, put, writing, t } = write;
  if (folded || !offered.includes("handled")) {
    return null;
  }
  return (
    <DispositionButton
      disposition="handled"
      writing={writing}
      onPress={() => put("handled")}
      t={t}
    />
  );
}

// The judgements that put the row down for now, first in the row's menu: Done
// below the fold, then the snooze and its longer answers. A swipe carries the
// same judgements for a thumb. These lines are the route a keyboard, a switch
// or a screen reader takes, and they mount on the menu's first open.
export function PutDownLines({ write }: Readonly<{ write: PutDown }>) {
  const folded = useFoldedViewport();
  const { offered, put, writing, t, locale } = write;
  const line = (disposition: WorklistDisposition) => (
    <DispositionButton
      disposition={disposition}
      writing={writing}
      onPress={() => put(disposition)}
      t={t}
    />
  );
  const snoozes = offered.includes("snooze");
  return (
    <>
      {folded && offered.includes("handled") && line("handled")}
      {snoozes && line("snooze")}
      {/* The LONGER SPANS, as lines of their own rather than behind a second
          control: a popover inside a menu is a second layer over a surface
          already floating. The plain Snooze above still sends the default
          day. Each line carries its own sentence, "Snooze for 3 days", because
          a bare "3 days" is a fragment whose subject a reader has to guess. */}
      {snoozes &&
        SNOOZE_SPANS.filter((days) => days !== SNOOZE_DAYS).map((days) => (
          <Button
            key={days}
            variant="ghost"
            {...writeState(writing, { disposition: "snooze", until: days })}
            onClick={() => put("snooze", days)}
          >
            {translatePlural(
              locale,
              "worklist.disposition.snoozeForDays",
              days,
              { value: formatNumber(days, locale) },
            )}
          </Button>
        ))}
      {/* The event answers after the spans: a rep reaching for a snooze most
          often still means a duration, and these add the case a duration
          cannot state. */}
      {snoozes &&
        SNOOZE_EVENTS.map((event) => (
          <Button
            key={event}
            variant="ghost"
            {...writeState(writing, { disposition: "snooze", until: event })}
            onClick={() => put("snooze", event)}
          >
            {t(`worklist.disposition.snoozeUntil.${event}` as const)}
          </Button>
        ))}
    </>
  );
}

// The judgements that say the row is not the reader's work. They come last in
// the menu, since they take the row off the list.
export function HandBackLines({ write }: Readonly<{ write: PutDown }>) {
  const { offered, put, writing, t } = write;
  return (
    <>
      {offered
        .filter(
          (disposition) =>
            disposition !== "handled" && disposition !== "snooze",
        )
        .map((disposition) => (
          <DispositionButton
            key={disposition}
            disposition={disposition}
            writing={writing}
            onPress={() => put(disposition)}
            t={t}
          />
        ))}
    </>
  );
}

// The two sides, built from what the SERVER offered rather than from the map.
//
// A judgement the server did not offer draws no gesture, the same rule the
// buttons follow — the client keeping its own idea of which rows a rep may
// judge is how a verb 404s or a rep loses one they were entitled to. Where a
// side has no offered judgement it is simply absent, and SwipeRow stages
// nothing that way.
export function swipeActions(
  offered: readonly WorklistDisposition[],
  t: T,
  put: (disposition: WorklistDisposition) => void,
  walked: Readonly<{ start: number; end: number }>,
) {
  const side = (which: "start" | "end") => {
    const here = offered.filter((one) => SWIPE_SIDES[one] === which);
    // Repeated swipes WALK the side rather than restaging the same judgement,
    // so a direction carrying two of them reaches both. Modulo rather than a
    // stop at the end: a reader who has swiped past what they wanted gets it
    // again on the next flick instead of a direction that stops answering.
    const one = here[walked[which] % here.length];
    if (one === undefined) {
      return undefined;
    }
    return {
      label: t(`worklist.disposition.verb.${one}` as const),
      onAct: () => put(one),
    };
  };
  return { start: side("start"), end: side("end") };
}

// What the confirmation says a press did.
//
// The span has to reach this sentence or the sentence is false. The default
// press really does bring the row back tomorrow, and the fixed copy said so —
// but the moment a reader could choose seven days, that same line told them
// "back tomorrow" for a row returning next week. A confirmation that misstates
// what just happened is worse than none: the reader believes it and stops
// checking.
//
// Only the snooze needs the span. The other judgements name no moment, so they
// keep the sentence they had.
function doneText(
  disposition: WorklistDisposition,
  until: SnoozeSpan | SnoozeEvent | undefined,
  source: WorklistItem["source"],
  t: T,
  locale: Locale,
): string {
  // Only a waiting message comes back when they write again. Any other row
  // comes back when something new happens on what it is about.
  if (disposition === "handled" && source !== "customer_waiting") {
    return t("worklist.disposition.done.handledRow");
  }
  if (disposition !== "snooze") {
    return t(`worklist.disposition.done.${disposition}` as const);
  }
  if (typeof until === "string") {
    return t(`worklist.disposition.doneSnoozeUntil.${until}` as const);
  }
  const span = until ?? SNOOZE_DAYS;
  return translatePlural(locale, "worklist.disposition.doneSnooze", span, {
    value: formatNumber(span, locale),
  });
}

// When a snooze lifts.
//
// Computed at the press rather than held in state: a page left open overnight
// would otherwise snooze until a moment that has already passed, and the server
// would refuse it with a validation error the reader cannot act on.
function snoozeUntil(days: number = SNOOZE_DAYS): Date {
  const until = new Date();
  until.setDate(until.getDate() + days);
  return until;
}
