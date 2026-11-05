// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type ComponentPropsWithRef,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { useDateTimePreferences } from "../format/preferences";
import {
  type ClockTime,
  formatClockTime,
  parseTypedTime,
  type TimePattern,
  timePattern,
  timePlaceholder,
} from "../format/timepattern";
import { useLocale, useT } from "../i18n";
import { isImeCommit } from "../lib/imecommit";
import { describedByIds } from "./describedby";
import { ErrorLine } from "./errorline";
import { firstRefusedControl } from "./formvalidity";

export {
  type ClockTime,
  clockOrBlank,
  isClockTime,
} from "../format/timepattern";

/** What `onChange` hears: the time as `HH:MM`, or `""` once cleared. */
export type TimeInputChange = Readonly<{
  target: Readonly<{ value: ClockTime | "" }>;
}>;

export type TimeInputProps = Omit<
  ComponentPropsWithRef<"input">,
  "type" | "value" | "defaultValue" | "onChange"
> &
  Readonly<{
    value: ClockTime | "";
    onChange?: (event: TimeInputChange) => void;
    /** Also takes `24:00`, for the end of a range that runs until midnight. */
    untilMidnight?: boolean;
  }>;

/**
 * The one time field. Its face follows the UI language and the installation's
 * time format, where a native `time` input follows the browser's. Typed text
 * reaches the caller only when the reader settles it: Enter, or leaving the box.
 */
export function TimeInput({
  value,
  onChange,
  untilMidnight = false,
  className,
  placeholder,
  onBlur,
  onKeyDown,
  onInvalid,
  ref,
  "aria-invalid": ariaInvalid,
  "aria-describedby": ariaDescribedBy,
  ...box
}: TimeInputProps) {
  const t = useT();
  const { locale } = useLocale();
  const { timeFormat } = useDateTimePreferences();
  const pattern = timePattern(locale, timeFormat);
  const shown = value === "" ? "" : formatClockTime(value, locale, pattern);
  const patternText = placeholderOf(pattern, t);
  // `null` while the box shows the value; the reader's own text once they type.
  const [draft, setDraft] = useState<string | null>(null);
  const [refused, setRefused] = useState(false);
  const input = useRef<HTMLInputElement | null>(null);
  const refusedId = useId();
  const problem =
    draft !== null &&
    draft.trim() !== "" &&
    !parseTypedTime(draft, pattern, untilMidnight)
      ? t("calendar.invalidTime", { pattern: patternText })
      : undefined;
  useLayoutEffect(() => {
    input.current?.setCustomValidity(problem ?? "");
  }, [problem]);

  const settle = () => {
    if (draft === null) {
      return;
    }
    const time =
      draft.trim() === "" ? "" : parseTypedTime(draft, pattern, untilMidnight);
    if (time === null) {
      setRefused(true);
      return;
    }
    setDraft(null);
    setRefused(false);
    if (time !== value) {
      onChange?.({ target: { value: time } });
    }
  };

  return (
    <>
      <input
        {...box}
        ref={(node) => {
          input.current = node;
          if (typeof ref === "function") {
            ref(node);
          } else if (ref) {
            ref.current = node;
          }
        }}
        type="text"
        inputMode="text"
        autoComplete="off"
        className={["input", className].filter(Boolean).join(" ")}
        placeholder={placeholder ?? patternText}
        style={{ minInlineSize: `${patternText.length + 2}ch`, ...box.style }}
        aria-invalid={refused ? true : ariaInvalid}
        aria-describedby={describedByIds(refused && refusedId, ariaDescribedBy)}
        value={draft ?? shown}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={(event) => {
          settle();
          onBlur?.(event);
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !isImeCommit(event)) {
            settle();
          }
          if (event.key === "Escape" && draft !== null && draft !== shown) {
            event.preventDefault();
            setDraft(null);
            setRefused(false);
          }
          onKeyDown?.(event);
        }}
        onInvalid={(event) => {
          if (problem !== undefined) {
            // Its ErrorLine says it; only a form's first refusal keeps the bubble.
            const field = event.currentTarget;
            if (!field.form || firstRefusedControl(field.form) !== field) {
              event.preventDefault();
            }
            setRefused(true);
          }
          onInvalid?.(event);
        }}
      />
      {refused && problem !== undefined && (
        <ErrorLine id={refusedId}>{problem}</ErrorLine>
      )}
    </>
  );
}

function placeholderOf(
  pattern: TimePattern,
  t: ReturnType<typeof useT>,
): string {
  return timePlaceholder(pattern, {
    hour: t("calendar.patternHour"),
    minute: t("calendar.patternMinute"),
  });
}
