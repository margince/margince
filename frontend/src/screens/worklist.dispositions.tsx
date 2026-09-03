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

import { Button } from "../design-system/atoms";
import { Popover } from "../design-system/popover";
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

// The set-aside verbs for one row, drawn only where the server offers them.
export function DispositionVerbs({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const set = useSetDisposition();
  const clear = useClearDisposition();
  const offered = item.dispositions ?? [];
  if (offered.length === 0) {
    return null;
  }
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
  return (
    <div className="worklist-row-dispositions">
      {offered.map((disposition) => (
        <Button
          key={disposition}
          small
          variant="ghost"
          disabled={set.isPending}
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
        <SnoozeSpans
          pending={set.isPending}
          onPick={(days) => put("snooze", days)}
        />
      )}
    </div>
  );
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
