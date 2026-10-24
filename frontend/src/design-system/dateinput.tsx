// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays } from "lucide-react";
import {
  type ComponentPropsWithRef,
  type Dispatch,
  type RefObject,
  type SetStateAction,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import {
  type DatePattern,
  datePattern,
  formatISODate,
  type ISODate,
  parseTypedDate,
  patternPlaceholder,
} from "../format/datepattern";
import { useDateTimePreferences } from "../format/preferences";
import { type Locale, useLocale, useT } from "../i18n";
import { isImeCommit } from "../lib/imecommit";
import { useAnchoredToTrigger } from "./anchored";
import { useDismissOnOutsidePress } from "./anchoredpopup";
import { Button } from "./button";
import { Calendar, dateOf, isoDay } from "./calendar";
import { describedByIds } from "./describedby";
import { useDialogFocus } from "./dialogfocus";
import { ErrorLine } from "./errorline";
import { firstRefusedControl } from "./formvalidity";
import { IconAction } from "./iconaction";
import "./dateinput.css";

export {
  type ISODate,
  isISODate,
  isoOrBlank,
} from "../format/datepattern";

/** What `onChange` hears: the day as `YYYY-MM-DD`, or `""` once cleared. */
export type DateInputChange = Readonly<{
  target: Readonly<{ value: ISODate | "" }>;
}>;

/**
 * The text box's own props, with the date narrowed to `YYYY-MM-DD`. `today` is the
 * day the calendar marks and Today picks: pass the day a `min`/`max` counts from.
 */
export type DateInputProps = Omit<
  ComponentPropsWithRef<"input">,
  "type" | "value" | "defaultValue" | "onChange" | "min" | "max"
> &
  Readonly<{
    value?: ISODate | "";
    defaultValue?: ISODate | "";
    onChange?: (event: DateInputChange) => void;
    min?: ISODate;
    max?: ISODate;
    today?: ISODate;
  }>;

// `holds` is the caller's value the text sits over: a move to any other value
// drops the text. `refused` is why a settle could not read it.
type Draft = Readonly<{
  text: string;
  holds: ISODate | "";
  refused?: string;
}>;

/**
 * The one date field. Typed text reaches the caller only when the reader
 * settles it: Enter, leaving the text box, or a pick in the calendar.
 */
export function DateInput({
  value,
  defaultValue,
  onChange,
  min,
  max,
  today,
  className,
  disabled,
  readOnly,
  name,
  placeholder,
  onBlur,
  onKeyDown,
  onInvalid,
  ref,
  "aria-invalid": ariaInvalid,
  "aria-describedby": ariaDescribedBy,
  ...box
}: DateInputProps) {
  const t = useT();
  const { locale } = useLocale();
  const { dateFormat } = useDateTimePreferences();
  const pattern = datePattern(locale, dateFormat);
  const [own, setOwn] = useState<ISODate | "">(defaultValue ?? "");
  const current = value ?? own;
  const [draft, setDraft] = useDraft(current);
  const [labelText, setLabelText] = useState("");
  const [openedOn, setOpenedOn] = useState<ISODate | null>(null);
  const shell = useRef<HTMLSpanElement | null>(null);
  const input = useRef<HTMLInputElement | null>(null);
  const popupId = useId();
  const refusedId = useId();
  const refusal = (day: ISODate) => outsideRange(day, min, max, pattern, t);
  const patternText = placeholderOf(pattern, t);
  const chooseName = calendarName(box["aria-label"] ?? labelText, t);

  const problem = draft
    ? readTyped(draft.text, pattern, refusal, t).problem
    : undefined;
  useLayoutEffect(() => {
    input.current?.setCustomValidity(problem ?? "");
  }, [problem]);

  const unsettled = draft !== null && draft.text !== shown(current, pattern);

  const report = (next: ISODate | "") => {
    if (value === undefined) {
      setOwn(next);
    }
    if (next !== current) {
      onChange?.({ target: { value: next } });
    }
  };
  // Cleared here, not by the effect above: the caller hears the day in this handler.
  const settle = (next: ISODate | "") => {
    input.current?.setCustomValidity("");
    setDraft(null);
    report(next);
  };
  const settleTyped = () => {
    if (!draft) {
      return;
    }
    const read = readTyped(draft.text, pattern, refusal, t);
    if (read.problem === undefined) {
      settle(read.day);
      return;
    }
    setDraft({ text: draft.text, holds: "", refused: read.problem });
    report("");
  };
  const type = (text: string) =>
    setDraft({ text, holds: draft ? draft.holds : current });

  return (
    <>
      <span
        ref={shell}
        className={["field-shell", "date-input", className]
          .filter(Boolean)
          .join(" ")}
      >
        <input
          {...box}
          ref={(node) => {
            input.current = node;
            if (node) {
              setLabelText(spokenLabel(node));
            }
            handOn(ref, node);
          }}
          type="text"
          className="input"
          autoComplete="off"
          disabled={disabled}
          readOnly={readOnly}
          placeholder={placeholder ?? patternText}
          style={{
            minInlineSize: `${patternText.length}ch`,
            ...box.style,
          }}
          aria-invalid={draft?.refused === undefined ? ariaInvalid : true}
          aria-describedby={describedByIds(
            draft?.refused !== undefined && refusedId,
            ariaDescribedBy,
          )}
          value={draft ? draft.text : shown(current, pattern)}
          onChange={(event) => type(event.target.value)}
          onBlur={(event) => {
            settleTyped();
            onBlur?.(event);
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !isImeCommit(event)) {
              settleTyped();
            }
            // The first Escape takes back typed text, as InlineText's does.
            // Marked handled, so a dialog around it hears only the next one.
            if (event.key === "Escape" && openedOn === null && unsettled) {
              event.preventDefault();
              setDraft(null);
            }
            onKeyDown?.(event);
          }}
          onInvalid={(event) => {
            if (problem !== undefined) {
              // Its ErrorLine says it, so the bubble is cancelled. The form's first
              // refusal keeps it: only an uncancelled one gets the browser's focus.
              const box = event.currentTarget;
              if (!box.form || firstRefusedControl(box.form) !== box) {
                event.preventDefault();
              }
              setDraft((held) =>
                held && held.refused === undefined
                  ? { ...held, refused: problem }
                  : held,
              );
            }
            onInvalid?.(event);
          }}
        />
        <IconAction
          label={chooseName}
          icon={<CalendarDays aria-hidden="true" />}
          disabled={disabled || readOnly}
          opensDialog
          disclosure={{ expanded: openedOn !== null, controls: popupId }}
          onClick={() =>
            setOpenedOn((opened) =>
              opened ? null : (today ?? isoDay(new Date())),
            )
          }
        />
        {name !== undefined && (
          <input
            type="hidden"
            name={name}
            value={current}
            disabled={disabled}
          />
        )}
        {openedOn && (
          <DatePopup
            id={popupId}
            name={chooseName}
            anchor={shell}
            selected={current}
            today={openedOn}
            locale={locale}
            refusal={refusal}
            onPick={(next) => {
              settle(next);
              setOpenedOn(null);
            }}
            onClose={() => setOpenedOn(null)}
          />
        )}
      </span>
      {draft?.refused !== undefined && (
        <ErrorLine id={refusedId}>{draft.refused}</ErrorLine>
      )}
    </>
  );
}

type Translate = ReturnType<typeof useT>;

function useDraft(
  current: ISODate | "",
): [Draft | null, Dispatch<SetStateAction<Draft | null>>] {
  const [draft, setDraft] = useState<Draft | null>(null);
  const [heard, setHeard] = useState(current);
  if (heard !== current) {
    setHeard(current);
    if (draft && draft.holds !== current) {
      setDraft(null);
    }
  }
  return [draft, setDraft];
}

function DatePopup({
  id,
  name,
  anchor,
  selected,
  today,
  locale,
  refusal,
  onPick,
  onClose,
}: Readonly<{
  id: string;
  name: string;
  anchor: RefObject<HTMLSpanElement | null>;
  selected: ISODate | "";
  today: ISODate;
  locale: Locale;
  refusal: (day: ISODate) => string | undefined;
  onPick: (next: ISODate | "") => void;
  onClose: () => void;
}>) {
  const t = useT();
  const panel = useRef<HTMLDivElement | null>(null);
  const [month, setMonth] = useState(() => dateOf(selected || today));
  const at = useAnchoredToTrigger(true, anchor, panel, "end");
  useDismissOnOutsidePress(true, onClose, anchor, panel);
  useDialogFocus({
    open: true,
    onClose,
    container: panel,
    initialFocusTo: () =>
      panel.current?.querySelector<HTMLElement>('[data-day][tabindex="0"]') ??
      null,
    returnFocusTo: () =>
      anchor.current?.querySelector<HTMLElement>("button") ?? null,
  });
  const todayRefused = refusal(today) !== undefined;

  return createPortal(
    <div
      ref={panel}
      id={id}
      role="dialog"
      aria-modal="true"
      aria-label={name}
      className="date-popup"
      style={{
        top: `${at.top}px`,
        left: `${at.left}px`,
        maxHeight: `${at.maxHeight}px`,
      }}
    >
      <Calendar
        month={month}
        onMonthChange={setMonth}
        selected={selected}
        onSelect={onPick}
        today={dateOf(today)}
        locale={locale}
        refusal={refusal}
      />
      <div className="date-popup-foot">
        <Button disabled={todayRefused} onClick={() => onPick(today)}>
          {t("calendar.today")}
        </Button>
        <Button onClick={() => onPick("")}>{t("calendar.clear")}</Button>
      </div>
    </div>,
    document.body,
  );
}

function handOn(ref: DateInputProps["ref"], node: HTMLInputElement | null) {
  if (typeof ref === "function") {
    ref(node);
  } else if (ref) {
    ref.current = node;
  }
}

function calendarName(field: string, t: Translate): string {
  return field
    ? t("calendar.chooseDateFor", { field })
    : t("calendar.chooseDate");
}

// The words a screen reader takes from the field's label; a required mark is
// `aria-hidden`, so it stays out.
function spokenLabel(box: HTMLInputElement): string {
  const label = box.labels?.[0];
  return label ? spokenText(label).trim() : "";
}

function spokenText(node: Node): string {
  if (node instanceof Element && node.getAttribute("aria-hidden") === "true") {
    return "";
  }
  if (node.nodeType === Node.TEXT_NODE) {
    return node.textContent ?? "";
  }
  return Array.from(node.childNodes, spokenText).join("");
}

function shown(day: ISODate | "", pattern: DatePattern): string {
  return day === "" ? "" : formatISODate(day, pattern);
}

function placeholderOf(pattern: DatePattern, t: Translate): string {
  return patternPlaceholder(pattern, {
    day: t("calendar.patternDay"),
    month: t("calendar.patternMonth"),
    year: t("calendar.patternYear"),
  });
}

function outsideRange(
  day: ISODate,
  min: ISODate | undefined,
  max: ISODate | undefined,
  pattern: DatePattern,
  t: Translate,
): string | undefined {
  if (min && day < min) {
    return t("calendar.earliest", { date: formatISODate(min, pattern) });
  }
  if (max && day > max) {
    return t("calendar.latest", { date: formatISODate(max, pattern) });
  }
  return undefined;
}

function readTyped(
  text: string,
  pattern: DatePattern,
  refusal: (day: ISODate) => string | undefined,
  t: Translate,
):
  | Readonly<{ day: ISODate | ""; problem?: undefined }>
  | Readonly<{ problem: string }> {
  if (text.trim() === "") {
    return { day: "" };
  }
  const day = parseTypedDate(text, pattern);
  if (day === null) {
    return {
      problem: t("calendar.invalidDate", {
        pattern: placeholderOf(pattern, t),
      }),
    };
  }
  const refused = refusal(day);
  return refused === undefined ? { day } : { problem: refused };
}
