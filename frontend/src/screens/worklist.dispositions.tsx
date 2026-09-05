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
// disappearance needs its way back offered at the moment of the disappearance;
// finding it again afterwards means knowing which of three judgements you made.

import { createContext, useContext, useState } from "react";

import { useFoldedViewport } from "../app/viewport";
import { Button, OverflowMenu } from "../design-system/atoms";
import { Popover } from "../design-system/popover";
import { SwipeRow } from "../design-system/swiperow";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { type Locale, translatePlural, useLocale, useT } from "../i18n";
import {
  useClearDisposition,
  useSetDisposition,
  type WorklistDisposition,
  type WorklistItem,
} from "./worklist.queries";

// How long a snooze lasts, when the reader does not say.
//
// One working day, because the verb answers "not now, but today is not over" —
// a rep reaching for it at ten in the morning means tomorrow morning, not this
// afternoon. It stays the default the plain press takes, so the fast path is
// still one click.
const SNOOZE_DAYS = 1;

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
const SNOOZE_SPANS = [1, 3, 7] as const;

type SnoozeSpan = (typeof SNOOZE_SPANS)[number];

// Which undo REACH each judgement takes back.
//
// `not_sales` bound the whole workspace, so its undo has to say `thread` or it
// would clear a reader-state row that was never written and leave the thread
// hidden from everybody. The two personal states clear the reader's own.
function undoScope(disposition: WorklistDisposition): "mine" | "thread" {
  return disposition === "not_sales" ? "thread" : "mine";
}

type T = ReturnType<typeof useT>;

// Which side of a swipe each judgement answers.
//
// `snooze` goes with the finger's forward direction because it is the one a rep
// reaches for most and the one they take back most: it puts the row down for a
// day rather than deciding anything about it. The two that say "this is not my
// work" go the other way, together, so the direction a reader learns is the
// direction that MEANS something rather than one verb's private habit.
//
// ONE SIDE EACH, and every judgement named.
//
// A gesture has two directions and the contract offers three judgements, so a
// map alone cannot place them: two sharing a side means the second is
// unreachable, which is the capability-lost-on-a-phone failure the row ceiling
// was already hiding. The side is therefore the ORDER a repeated swipe walks —
// `swipeActions` cycles the judgements on a side rather than taking the first —
// and this map decides which walk a direction starts.
//
// `snooze` leads the forward walk because it is the one a rep reaches for most
// and takes back most: it puts the row down for a day rather than deciding
// anything about it. The two that say "this is not my work" share the other
// direction, in the order a reader meets them.
//
// EXHAUSTIVE rather than partial: `Record` so a fourth disposition added to the
// contract fails the build here, instead of shipping a verb no phone can reach.
export const SWIPE_SIDES = {
  snooze: "end",
  not_mine: "start",
  not_sales: "start",
} as const satisfies Record<WorklistDisposition, "start" | "end">;

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
  const offered = item.dispositions ?? [];
  const put = (disposition: WorklistDisposition, days?: SnoozeSpan) => {
    set.mutate(
      {
        activityId: item.id,
        disposition,
        ...(disposition === "snooze"
          ? { snoozedUntil: snoozeUntil(days).toISOString() }
          : {}),
      },
      {
        onSuccess: () =>
          toast.show(doneText(disposition, days, t, locale), {
            action: {
              label: t("worklist.disposition.undo"),
              // A failed undo needs saying. The toast dismisses itself the
              // moment the action is pressed, so without this the row stays
              // hidden and the only way back has just left the screen.
              onAct: () =>
                clear.mutate(
                  { activityId: item.id, scope: undoScope(disposition) },
                  {
                    onError: () =>
                      toast.show(t("worklist.disposition.undoFailed"), {
                        mark: false,
                      }),
                  },
                ),
            },
          }),
        onError: () =>
          toast.show(t("worklist.disposition.failed"), { mark: false }),
      },
    );
  };
  return { offered, put, pending: set.isPending, t };
}

// The whole row, answerable with the thumb where there is no width for verbs.
//
// It wraps the ROW rather than the verbs, and that is the difference between a
// feature and a demo: `.worklist-row` is a flex container and its verbs are one
// item in it, so a gesture mounted around them would ask a reader to swipe an
// 83px chip and would draw an empty box on a row whose only judgements are the
// ones the chip does not carry.
//
// Above the fold it draws the row and nothing else. The buttons are still the
// row's own, drawn beside the work by DispositionVerbs, and this adds nothing
// they do not already offer.
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

type PutDown = ReturnType<typeof usePutDown>;

// The same judgements, reachable without a pointer.
//
// A swipe is not a control a keyboard, a switch or a screen reader can operate,
// so below the fold the three judgements had exactly one route and it needed a
// finger. The staged bar the gesture opens carries real buttons, but only a
// drag opens it.
//
// A MENU rather than the buttons back. Restoring them is the 44px band that was
// removed to bring the row under its ceiling, so the fix would undo the change
// it is fixing; a menu is one tab stop and one line of text, and it is what the
// design system already offers for "the verbs a record offers but a reader
// rarely wants". The swipe stays the fast path for a thumb and this is the one
// that answers to a key.
//
// Its children mount on first open, which is the default and matters here: a
// queue draws one of these per row.
//
// It takes the WRITE rather than opening its own. A second usePutDown on the
// same row is a second mutation with its own `pending`, so a press here and a
// swipe confirm beside it both fire: two writes for one judgement, and two
// toasts each offering an Undo for a state the other already cleared.
function PutDownMenu({
  offered,
  put,
  pending,
  t,
}: Readonly<{
  offered: readonly WorklistDisposition[];
  put: (disposition: WorklistDisposition) => void;
  pending: boolean;
  t: T;
}>) {
  return (
    <OverflowMenu label={t("worklist.disposition.menu")}>
      {offered.map((disposition) => (
        <Button
          key={disposition}
          small
          variant="ghost"
          // PENDING, never disabled. A disabled control leaves the tab order,
          // so a keyboard reader who presses one is dropped to the body — the
          // exact failure this menu exists to fix. The atom refuses the press
          // through aria-disabled and keeps the control reachable.
          pending={pending}
          onClick={() => put(disposition)}
        >
          {t(`worklist.disposition.verb.${disposition}` as const)}
        </Button>
      ))}
    </OverflowMenu>
  );
}

// The set-aside verbs, drawn where there is width for them.
//
// Below the fold they are GONE rather than smaller, for the width arithmetic in
// design-system/swiperow.tsx's header. PutDownByThumb carries every JUDGEMENT
// there, on the row itself.
//
// The snooze SPANS do not survive the fold, and that is a gap rather than a
// design. The gesture sends the default day, so below 720px a rep who knows a
// customer is away all week can no longer say so — they press again tomorrow,
// which is the state the spans were added to end. Restoring them means another
// control in the row, which is the 44px band this change removed; that trade is
// a product decision, filed as issue #4313 rather than settled here.

export function DispositionVerbs({ item }: Readonly<{ item: WorklistItem }>) {
  const folded = useFoldedViewport();
  // The row's own write, from the wrapper that holds it — one mutation per row,
  // so a press here and a swipe confirm beside it cannot write twice.
  //
  // The fallback is for the callers that draw these verbs OUTSIDE a row, the
  // Brief's Do Next section among them: they get their own write rather than
  // throwing on a missing provider. Both hooks run either way, because a hook
  // cannot be called conditionally; the unused one registers a mutation nobody
  // fires, which costs a registration and never a request.
  const shared = useContext(PutDownContext);
  const own = usePutDown(item);
  const { offered, put, pending, t } = shared ?? own;
  if (offered.length === 0) {
    return null;
  }
  // Below the fold the band goes and the menu stands in its place: one tab stop
  // rather than four 44px controls, so the judgements stay reachable by key
  // without the height that put the row over its ceiling.
  if (folded) {
    return <PutDownMenu offered={offered} put={put} pending={pending} t={t} />;
  }
  return (
    <div className="worklist-row-dispositions">
      {offered.map((disposition) => (
        <Button
          key={disposition}
          small
          variant="ghost"
          disabled={pending}
          onClick={() => put(disposition)}
        >
          {t(`worklist.disposition.verb.${disposition}` as const)}
        </Button>
      ))}
      {/* The spans, BESIDE the snooze verb rather than replacing it. A rep
          reaching for "not today" most often means tomorrow, and making them
          choose a duration every time would charge the common case for the
          rare one. */}
      {offered.includes("snooze") && (
        <SnoozeSpans pending={pending} onPick={(days) => put("snooze", days)} />
      )}
    </div>
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
    if (here.length === 0) {
      return undefined;
    }
    // Repeated swipes WALK the side rather than restaging the same judgement,
    // so a direction carrying two of them reaches both. Modulo rather than a
    // stop at the end: a reader who has swiped past what they wanted gets it
    // again on the next flick instead of a direction that stops answering.
    const one = here[walked[which] % here.length];
    return {
      label: t(`worklist.disposition.verb.${one}` as const),
      onAct: () => put(one),
    };
  };
  return { start: side("start"), end: side("end") };
}

/**
 * How long to put a row down for, when a day is not the answer.
 *
 * The server has always taken any future instant; this file was the thing that
 * only ever sent tomorrow. A rep who knows a customer is away all week pressed
 * the same button seven mornings running, and each press was a row that came
 * back and a count that stayed wrong.
 *
 * A Popover rather than a row of buttons: three more controls on every snoozable
 * row is three more things to read past on a page whose whole argument is that
 * it can be worked to the bottom.
 */
function SnoozeSpans({
  pending,
  onPick,
}: Readonly<{ pending: boolean; onPick: (days: SnoozeSpan) => void }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Popover
      label={t("worklist.disposition.snoozeFor")}
      className="link-button"
    >
      <div className="worklist-snooze-spans">
        {SNOOZE_SPANS.map((days) => (
          <Button
            key={days}
            small
            variant="ghost"
            disabled={pending}
            onClick={() => onPick(days)}
          >
            {translatePlural(locale, "worklist.disposition.snoozeDays", days, {
              value: formatNumber(days, locale),
            })}
          </Button>
        ))}
      </div>
    </Popover>
  );
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
  days: SnoozeSpan | undefined,
  t: T,
  locale: Locale,
): string {
  if (disposition !== "snooze") {
    return t(`worklist.disposition.done.${disposition}` as const);
  }
  const span = days ?? SNOOZE_DAYS;
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
