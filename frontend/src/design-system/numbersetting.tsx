// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import { isImeCommit } from "../lib/imecommit";
import { TextInput } from "./atoms";
import { describedByIds } from "./describedby";
import { ErrorLine } from "./errorline";
import { useSaveDraft } from "./savepage";
import { type SettingControlProps, SettingRow } from "./settingrow";
import "./numbersetting.css";

/**
 * A whole number an admin sets inside a range, in a `SettingRow`'s right
 * column. Anything else stays in the box with `refusal` under it, so the
 * reader corrects what they typed instead of retyping it.
 *
 * Inside a `SavePageProvider` the typed number is a draft the page's save bar
 * commits; elsewhere it commits on Enter or blur. `onCommit` may return the
 * write's promise, so the bar can keep a refused draft.
 *
 * `refusal` is the caller's sentence because only the caller knows the unit.
 * It is added to the row's description, so a reader hears the rule and how
 * they broke it.
 *
 * `lowestOn` is for a setting whose `min` of 0 means off. A value between 0
 * and `lowestOn` is refused too.
 *
 * Typing the stored value back is no edit. A draft is held against the value
 * it was typed over. The box shows the stored value again when it moves (a
 * save, a refetch, a colleague's change) instead of writing the old number back.
 */
export function NumberSetting({
  control,
  value,
  min,
  max,
  lowestOn,
  refusal,
  reading,
  onCommit,
  disabled,
  reason,
  describedBy,
  testId,
}: Readonly<{
  control: SettingControlProps;
  value: number;
  min: number;
  max: number;
  lowestOn?: number;
  refusal: string;
  /** The typed number in words under the box, for a unit a bare figure hides. */
  reading?: (value: number) => string;
  onCommit: (next: number) => unknown;
  disabled?: boolean;
  /** Why this reader may not change it, under the box; refuses the edit alone. */
  reason?: ReactNode;
  /**
   * An element elsewhere that describes this number, named after the row's
   * own description.
   */
  describedBy?: string;
  testId?: string;
}>) {
  const refusalId = useId();
  const readingId = useId();
  const reasonId = useId();
  const number = useNumberDraft(value, { min, max, lowestOn }, onCommit);
  const words = reading && number.typed !== null ? reading(number.typed) : null;
  return (
    <div className="settingrow-measure numbersetting">
      <TextInput
        {...control}
        aria-describedby={describedByIds(
          control["aria-describedby"],
          describedBy,
          words && readingId,
          reason !== undefined && reasonId,
          number.refused && refusalId,
        )}
        aria-invalid={number.refused ? true : undefined}
        className="t-num"
        data-testid={testId}
        inputMode="numeric"
        value={number.shown}
        disabled={disabled || reason !== undefined}
        onChange={(e) => number.type(e.target.value)}
        onBlur={() => number.commit("blur")}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !isImeCommit(e)) {
            e.preventDefault();
            number.commit("enter");
          }
        }}
      />
      {words && (
        <span id={readingId} className="t-caption numbersetting-reading">
          {words}
        </span>
      )}
      {reason !== undefined && (
        <p id={reasonId} className="t-caption numbersetting-reason">
          {reason}
        </p>
      )}
      <ErrorLine id={refusalId}>{number.refused ? refusal : null}</ErrorLine>
    </div>
  );
}

type Bounds = Readonly<{ min: number; max: number; lowestOn?: number }>;

// `sent` is the number this draft already committed. Without it the blur after
// an Enter would send it twice while the first save is in flight. A saved
// draft must not hold the page's bar open.
function useNumberDraft(
  value: number,
  bounds: Bounds,
  onCommit: (next: number) => unknown,
) {
  const [draft, setDraft] = useState<{
    text: string;
    over: number;
    sent?: number;
  } | null>(null);
  const [refused, setRefused] = useState(false);
  const live = draft !== null && draft.over === value ? draft : null;
  const shown = live?.text ?? String(value);
  const typed = acceptedNumber(shown, bounds);
  const changed =
    live !== null &&
    (live.text.trim() === "" || Number(live.text.trim()) !== value);
  const inPage = useSaveDraft({
    dirty: changed && live?.sent === undefined,
    invalid: typed === null,
    save: async () => {
      if (live === null || typed === null) return;
      await onCommit(typed);
      setDraft({ ...live, sent: typed });
    },
    discard: () => {
      setDraft(null);
      setRefused(false);
    },
  });
  const commit = (by: "enter" | "blur") => {
    if (live === null || !changed) {
      if (live !== null) setDraft(null);
      setRefused(false);
      return;
    }
    setRefused(typed === null);
    // In a page the bar sends the draft; here the reader only learns it is wrong.
    // Enter always sends, so a refused save can be retried.
    if (inPage || typed === null) return;
    if (by === "blur" && live.sent === typed) return;
    setDraft({ ...live, sent: typed });
    onCommit(typed);
  };
  return {
    shown,
    typed,
    refused,
    type: (text: string) => setDraft({ text, over: value }),
    commit,
  };
}

// Digits only: `Number` would also read "1e3", "0x10" and " -5 " as numbers.
function acceptedNumber(
  text: string,
  {
    min,
    max,
    lowestOn,
  }: Readonly<{ min: number; max: number; lowestOn?: number }>,
): number | null {
  const trimmed = text.trim();
  if (!/^\d+$/.test(trimmed)) {
    return null;
  }
  const next = Number(trimmed);
  if (next < min || next > max) {
    return null;
  }
  return lowestOn !== undefined && next !== 0 && next < lowestOn ? null : next;
}

/**
 * A `SettingRow` whose answer is a `NumberSetting` — the whole row, for the
 * common case where the row's label and description are all it needs.
 */
export function NumberSettingRow({
  label,
  description,
  ...number
}: Readonly<
  { label: string; description: string } & Omit<
    Parameters<typeof NumberSetting>[0],
    "control"
  >
>) {
  return (
    <SettingRow
      label={label}
      description={description}
      control={(control) => <NumberSetting control={control} {...number} />}
    />
  );
}
