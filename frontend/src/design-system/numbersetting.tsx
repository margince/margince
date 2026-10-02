// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { TextInput } from "./atoms";
import { ErrorLine } from "./errorline";
import type { SettingControlProps } from "./settingrow";
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
 * Typing the stored value back is no edit at all. A draft that a save then
 * stores reads the same as the stored value, so nothing has to clear it.
 */
export function NumberSetting({
  control,
  value,
  min,
  max,
  refusal,
  onCommit,
  disabled,
  testId,
}: Readonly<{
  control: SettingControlProps;
  value: number;
  min: number;
  max: number;
  refusal: string;
  onCommit: (next: number) => void;
  disabled?: boolean;
  testId?: string;
}>) {
  const [draft, setDraft] = useState<string | null>(null);
  const [refused, setRefused] = useState(false);
  const refusalId = useId();
  const shown = draft ?? String(value);
  const commit = () => {
    const next = Number(shown.trim());
    if (next === value) {
      setDraft(null);
      setRefused(false);
      return;
    }
    if (
      shown.trim() === "" ||
      !Number.isInteger(next) ||
      next < min ||
      next > max
    ) {
      setRefused(true);
      return;
    }
    setRefused(false);
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
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            commit();
          }
        }}
      />
      <ErrorLine id={refusalId}>{refused ? refusal : null}</ErrorLine>
    </div>
  );
}
