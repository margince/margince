// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { TextInput } from "./atoms";
import { ErrorLine } from "./errorline";
import { type SettingControlProps, SettingRow } from "./settingrow";
import "./numbersetting.css";

/**
 * A whole number an admin sets inside a range, in a `SettingRow`'s right
 * column. It commits on Enter or blur like a rename, and only a whole number in
 * range reaches `onCommit`: anything else stays in the box, with `refusal`
 * under it, so the reader corrects what they typed instead of retyping it.
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
 * than a draft that would write the old number back on the next blur.
 */
export function NumberSetting({
  control,
  value,
  min,
  max,
  lowestOn,
  refusal,
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
  onCommit: (next: number) => void;
  disabled?: boolean;
  testId?: string;
}>) {
  // `sent` is the number this draft already committed: the blur that follows
  // an Enter would otherwise send it a second time while the first save is in
  // flight. Enter itself always commits, so a refused save can be retried.
  const [draft, setDraft] = useState<{
    text: string;
    over: number;
    sent?: number;
  } | null>(null);
  const [refused, setRefused] = useState(false);
  const refusalId = useId();
  const live = draft !== null && draft.over === value ? draft : null;
  const shown = live?.text ?? String(value);
  const commit = (by: "enter" | "blur") => {
    if (live === null) {
      setRefused(false);
      return;
    }
    const next = Number(live.text.trim());
    if (next === value) {
      setDraft(null);
      setRefused(false);
      return;
    }
    if (
      live.text.trim() === "" ||
      !Number.isInteger(next) ||
      next < min ||
      next > max ||
      (lowestOn !== undefined && next !== 0 && next < lowestOn)
    ) {
      setRefused(true);
      return;
    }
    setRefused(false);
    if (by === "blur" && live.sent === next) {
      return;
    }
    setDraft({ ...live, sent: next });
    onCommit(next);
  };
  return (
    <div className="settingrow-measure numbersetting">
      <TextInput
        {...control}
        aria-describedby={
          [control["aria-describedby"], refused ? refusalId : null]
            .filter(Boolean)
            .join(" ") || undefined
        }
        aria-invalid={refused ? true : undefined}
        data-testid={testId}
        inputMode="numeric"
        value={shown}
        disabled={disabled}
        onChange={(e) => setDraft({ text: e.target.value, over: value })}
        onBlur={() => commit("blur")}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            commit("enter");
          }
        }}
      />
      <ErrorLine id={refusalId}>{refused ? refusal : null}</ErrorLine>
    </div>
  );
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
