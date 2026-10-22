// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { TextInput } from "./atoms";
import { ErrorLine } from "./errorline";
import { useSaveDraft } from "./savepage";
import { type SettingControlProps, SettingRow } from "./settingrow";
import "./numbersetting.css";

/**
 * A whole number an admin sets inside a range, in a `SettingRow`'s right
 * column. Only a whole number in range is ever sent: anything else stays in the
 * box, with `refusal` under it, so the reader corrects what they typed instead
 * of retyping it.
 *
 * Inside a `SavePageProvider` the typed number is a draft the page's save bar
 * commits; elsewhere it commits on Enter or blur like a rename. `onCommit` may
 * return the write's promise, so the bar can keep a refused draft.
 *
 * `refusal` is the caller's sentence because only the caller knows the unit —
 * "15 to 10,080 minutes" says what a bare "15 to 10,080" cannot. The row's own
 * description stays on the field and the refusal is ADDED to it, so a reader
 * hears the rule and how they broke it.
 *
 * `lowestOn` is for a setting whose `min` of 0 means off: a value between 0
 * and `lowestOn` is refused too, so 0 and `lowestOn..max` are the choices.
 *
 * Typing the stored value back is no edit at all. A draft is held against the
 * value it was typed over, so the box shows the stored value again the moment
 * that value moves — a save landing, a refetch, a colleague's change — rather
 * than a draft that would write the old number back.
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
  testId?: string;
}>) {
  const refusalId = useId();
  const readingId = useId();
  const number = useNumberDraft(value, { min, max, lowestOn }, onCommit);
  const words = reading && number.typed !== null ? reading(number.typed) : null;
  return (
    <div className="settingrow-measure numbersetting">
      <TextInput
        {...control}
        aria-describedby={
          [
            control["aria-describedby"],
            words ? readingId : null,
            number.refused ? refusalId : null,
          ]
            .filter(Boolean)
            .join(" ") || undefined
        }
        aria-invalid={number.refused ? true : undefined}
        className="t-num"
        data-testid={testId}
        inputMode="numeric"
        value={number.shown}
        disabled={disabled}
        onChange={(e) => number.type(e.target.value)}
        onBlur={() => number.commit("blur")}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
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
      <ErrorLine id={refusalId}>{number.refused ? refusal : null}</ErrorLine>
    </div>
  );
}

type Bounds = Readonly<{ min: number; max: number; lowestOn?: number }>;

// `sent` is the number this draft already committed: the blur that follows an
// Enter would otherwise send it a second time while the first save is in
// flight, and a saved draft must not hold the page's bar open.
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
