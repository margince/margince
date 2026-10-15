// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The set-aside verbs a Worklist row can draw. This file holds which ones this
// build knows, their swipe sides, one verb as a button, and its write state.
// The band that places them is worklist.dispositions.tsx.

import { Button } from "../../design-system/atoms";
import { useTooltip } from "../../design-system/tooltip";
import type { useT } from "../../i18n";
import type { SnoozeEvent, SnoozeSpan } from "./worklist.dispositions";
import type { WorklistDisposition } from "./worklist.queries";

// Which answer a press sent, as opposed to the fact that one is out.
//
// The judgements on a row are alternatives, so the band tells the control that
// started a write from the ones standing down. The disposition alone cannot:
// three snooze answers send it with different moments behind it.
export type PutDownAnswer = Readonly<{
  disposition: WorklistDisposition;
  until?: SnoozeSpan | SnoozeEvent;
}>;

// The two states one in-flight write puts the band into, never both on one
// control.
//
// The control the reader pressed goes `pending`. `Button` refuses the second
// press through `aria-disabled` and keeps focus, so the wait is announced where
// the reader stands. Every other answer goes `disabled`: it started nothing.
// `DecisionVerbs` draws the same division (design-system/decisioncard.tsx).
//
// Never both, because inside the atom `disabled` outranks `pending`. A browser
// drops focus off a control that disables under the press, which `pending`
// exists to prevent.
export function writeState(
  writing: PutDownAnswer | undefined,
  answer: PutDownAnswer,
): { pending?: boolean; disabled?: boolean } {
  if (writing === undefined) {
    return {};
  }
  return writing.disposition === answer.disposition &&
    writing.until === answer.until
    ? { pending: true }
    : { disabled: true };
}

// The judgements whose label alone does not say what happens. "Not mine" could
// mean the contact or the reply; the hint says it leaves only this list.
const VERB_HINTS: Partial<
  Record<WorklistDisposition, "worklist.disposition.hint.not_mine">
> = { not_mine: "worklist.disposition.hint.not_mine" };

// One judgement as a button, with its hint on hover and focus where it has one.
export function DispositionButton({
  disposition,
  writing,
  onPress,
  t,
}: Readonly<{
  disposition: WorklistDisposition;
  writing: PutDownAnswer | undefined;
  onPress: () => void;
  t: ReturnType<typeof useT>;
}>) {
  const hint = VERB_HINTS[disposition];
  const { ref, trigger, tip } = useTooltip<HTMLButtonElement>(
    hint ? t(hint) : "",
  );
  return (
    <Button
      variant="ghost"
      {...writeState(writing, { disposition })}
      {...(hint ? { ref, ...trigger } : {})}
      onClick={onPress}
    >
      {t(`worklist.disposition.verb.${disposition}` as const)}
      {hint && tip}
    </Button>
  );
}

// Which side of a swipe each judgement answers.
//
// `handled` and `snooze` go with the finger's forward direction. Both put the
// reader's own work down, one for good and one for a while. The two that say
// "this is not my work" go the other way, together. The direction a reader
// learns then means something, rather than being one verb's habit.
//
// One side each, and every judgement named.
//
// A gesture has two directions and the contract offers four judgements, so a
// map alone cannot place them. Two sharing a side would leave the second
// unreachable on a phone. So the side is the order a repeated swipe walks:
// `swipeActions` cycles the judgements on a side rather than taking the first.
// This map decides which walk a direction starts.
//
// Each walk follows the order the server sends, so `handled` leads the forward
// walk as it leads the verbs. The two that say "this is not my work" share the
// other direction, in the order a reader meets them.
//
// EXHAUSTIVE rather than partial: `Record` so a fourth disposition added to the
// contract fails the build here, instead of shipping a verb no phone can reach.
export const SWIPE_SIDES = {
  handled: "end",
  snooze: "end",
  not_mine: "start",
  not_sales: "start",
} as const satisfies Record<WorklistDisposition, "start" | "end">;

// The dispositions this build can draw. A newer server may send a verb this
// tab has no label or swipe side for, and drawing it would show a raw key.
export function knownDispositions(
  sent: readonly string[] | undefined,
): WorklistDisposition[] {
  return (sent ?? []).filter((one): one is WorklistDisposition =>
    Object.hasOwn(SWIPE_SIDES, one),
  );
}
