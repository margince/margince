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
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import {
  useClearDisposition,
  useSetDisposition,
  type WorklistDisposition,
  type WorklistItem,
} from "./worklist.queries";

// How long a snooze lasts. One working day, because the verb answers "not now,
// but today is not over" — a rep reaching for it at ten in the morning means
// tomorrow morning, not this afternoon.
const SNOOZE_DAYS = 1;

// Which undo REACH each judgement takes back.
//
// `not_sales` bound the whole workspace, so its undo has to say `thread` or it
// would clear a reader-state row that was never written and leave the thread
// hidden from everybody. The two personal states clear the reader's own.
function undoScope(disposition: WorklistDisposition): "mine" | "thread" {
  return disposition === "not_sales" ? "thread" : "mine";
}

// The set-aside verbs for one row, drawn only where the server offers them.
export function DispositionVerbs({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const toast = useToast();
  const set = useSetDisposition();
  const clear = useClearDisposition();
  const offered = item.dispositions ?? [];
  if (offered.length === 0) {
    return null;
  }
  const put = (disposition: WorklistDisposition) => {
    set.mutate(
      {
        activityId: item.id,
        disposition,
        ...(disposition === "snooze"
          ? { snoozedUntil: snoozeUntil().toISOString() }
          : {}),
      },
      {
        onSuccess: () =>
          toast.show(t(`worklist.disposition.done.${disposition}` as const), {
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
    </div>
  );
}

// When a snooze lifts.
//
// Computed at the press rather than held in state: a page left open overnight
// would otherwise snooze until a moment that has already passed, and the server
// would refuse it with a validation error the reader cannot act on.
function snoozeUntil(): Date {
  const until = new Date();
  until.setDate(until.getDate() + SNOOZE_DAYS);
  return until;
}
