// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { openingCase } from "../format/collate";
import { datePattern, formatISODate, isISODate } from "../format/datepattern";
import { formatMoney, formatNumber } from "../format/format";
import { numberFormat } from "../format/formatters";
import { dateTimePreferences } from "../format/preferences";
import { type Locale, translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { FieldProblem } from "./problembody";
import { PROBLEM_CODE_COPY } from "./problemcodecopy";

// A field's name on screen. `limit` shows a wire bound in the unit the reader
// typed: minor units as money, a percent with its sign.
export type FieldLabel =
  | string
  | Readonly<{ label: string; limit?: (wire: string) => string }>;

// The words a screen holds for the fields a refusal can name, keyed by the
// field's wire path.
export type FieldLabels = Readonly<Record<string, FieldLabel>>;

type Wording = Readonly<{ named: MessageKey; bare: MessageKey }>;

const INVALID: Wording = {
  named: "common.fieldInvalidNamed",
  bare: "common.fieldInvalid",
};
const EMAIL: Wording = {
  named: "common.fieldNotEmail",
  bare: "common.valueNotEmail",
};
const IN_SET: Wording = {
  named: "common.fieldNotInSet",
  bare: "common.valueNotInSet",
};

// The field codes a screen words itself, with and without the field's label.
// The server's message for them is written for API callers.
const FIELD_CODE_WORDING = new Map<string, Wording>([
  [
    "required",
    { named: "common.fieldRequiredNamed", bare: "record.fieldRequired" },
  ],
  [
    "not_integer",
    { named: "common.fieldNotInteger", bare: "common.valueNotInteger" },
  ],
  [
    "not_number",
    { named: "common.fieldNotNumber", bare: "common.valueNotNumber" },
  ],
  ["email_malformed", EMAIL],
  ["invalid_email", EMAIL],
  [
    "invalid_date",
    { named: "common.fieldNotDate", bare: "common.valueNotDate" },
  ],
  [
    "datetime_malformed",
    { named: "common.fieldNotDateTime", bare: "common.valueNotDateTime" },
  ],
  ["invalid_enum", IN_SET],
  ["not_a_known_value", IN_SET],
  ["invalid_uuid", INVALID],
  ["invalid_type", INVALID],
  [
    "duplicate",
    { named: "common.fieldRepeated", bare: "common.valueRepeated" },
  ],
  [
    "invalid_characters",
    { named: "common.fieldOneLine", bare: "common.valueOneLine" },
  ],
  [
    "future",
    { named: "common.fieldDateInFuture", bare: "common.dateInFuture" },
  ],
  [
    "too_many_decimals",
    { named: "common.fieldDecimalPlaces", bare: "common.valueDecimalPlaces" },
  ],
  [
    "amount_negative",
    { named: "create.amountNegative", bare: "create.amountNegative" },
  ],
  [
    "reference_not_found",
    { named: "create.referenceNotFound", bare: "create.referenceNotFound" },
  ],
]);

// An out_of_range refusal words the bounds it states; an older emitter states
// none.
const RANGE_WORDING = {
  both: { named: "common.fieldBetween", bare: "common.valueBetween" },
  min: { named: "common.fieldAtLeast", bare: "common.valueAtLeast" },
  max: { named: "common.fieldAtMost", bare: "common.valueAtMost" },
  none: { named: "common.fieldOutOfRange", bare: "common.valueOutOfRange" },
} as const satisfies Record<string, Wording>;

const OUT_OF_RANGE = "out_of_range";
const TOO_LONG = "too_long";

// Whether an English reader gets catalog words for the fault. A field the screen
// does not label keeps the server's sentence, unless it opens on the wire key.
export function wordsFaultInEnglish(
  fault: FieldProblem,
  labels?: FieldLabels,
): boolean {
  if (PROBLEM_CODE_COPY.has(fault.code) || fault.code === TOO_LONG) return true;
  if (opensOnWireKey(fault)) return true;
  const worded =
    FIELD_CODE_WORDING.has(fault.code) || fault.code === OUT_OF_RANGE;
  return worded && labels?.[fault.field] !== undefined;
}

// A server sentence that opens on the wire key, backticked or bare, would show
// the reader a name from the code.
function opensOnWireKey(fault: FieldProblem): boolean {
  const message = fault.message.trimStart();
  if (message.startsWith("`")) return true;
  const firstWord = message.split(/\s/, 1)[0]?.replace(/[:,;.]$/, "");
  return firstWord !== undefined && firstWord === fault.field;
}

// A field fault in the reader's words, for the callout and for the line under
// the field alike. It opens a line and ends in a full stop.
export function faultSentence(
  fault: FieldProblem,
  locale: Locale,
  labels?: FieldLabels,
): string {
  const key = PROBLEM_CODE_COPY.get(fault.code);
  const label = labels?.[fault.field];
  const serverWords =
    locale === "en" && !wordsFaultInEnglish(fault, labels) && fault.message;
  const message = key
    ? translate(locale, key)
    : serverWords ||
      (codeMessage(fault, locale, label) ?? worded(INVALID, locale, label));
  const sentence = openingCase(message, locale);
  return /[.!?]$/.test(sentence) ? sentence : `${sentence}.`;
}

// A form's own check before it sends, in the words the server's refusal of
// the same value would get: `code` is a wire code, `params` its values.
export function fieldCheckSentence(
  code: string,
  locale: Locale,
  label?: FieldLabel,
  params?: Readonly<Record<string, string>>,
): string {
  const field = "";
  return faultSentence(
    { field, code, message: "", ...(params ? { params } : null) },
    locale,
    label === undefined ? undefined : { [field]: label },
  );
}

function codeMessage(
  fault: FieldProblem,
  locale: Locale,
  label: FieldLabel | undefined,
): string | null {
  if (fault.code === TOO_LONG) return tooLongMessage(fault, locale, label);
  if (fault.code === OUT_OF_RANGE) return rangeMessage(fault, locale, label);
  const wording = FIELD_CODE_WORDING.get(fault.code);
  if (!wording) return null;
  return worded(wording, locale, label, fault.params);
}

function rangeMessage(
  fault: FieldProblem,
  locale: Locale,
  label: FieldLabel | undefined,
): string {
  const min = fault.params?.min;
  const max = fault.params?.max;
  const shown = (wire: string) => limitText(wire, locale, label);
  const params = {
    ...(min === undefined ? null : { min: shown(min) }),
    ...(max === undefined ? null : { max: shown(max) }),
  };
  const wording =
    min !== undefined && max !== undefined
      ? RANGE_WORDING.both
      : min !== undefined
        ? RANGE_WORDING.min
        : max !== undefined
          ? RANGE_WORDING.max
          : RANGE_WORDING.none;
  return worded(wording, locale, label, params);
}

// A server older than the limit on the wire sends no max, and the copy then
// says "too long" without a number.
function tooLongMessage(
  fault: FieldProblem,
  locale: Locale,
  label: FieldLabel | undefined,
): string {
  if (fault.max === undefined) {
    return translate(locale, "common.valueTooLong");
  }
  return worded(
    { named: "common.fieldTooLongMax", bare: "common.valueTooLongMax" },
    locale,
    label,
    { max: formatNumber(fault.max, locale) },
  );
}

function worded(
  wording: Wording,
  locale: Locale,
  label: FieldLabel | undefined,
  params: Readonly<Record<string, string>> = {},
): string {
  const field = typeof label === "string" ? label : label?.label;
  return field
    ? translate(locale, wording.named, { ...params, field })
    : translate(locale, wording.bare, params);
}

// A bound as the reader reads it, in the screen's own unit when it has one.
// Else a day takes a date field's pattern and a number the reader's grouping.
function limitText(
  wire: string,
  locale: Locale,
  label: FieldLabel | undefined,
): string {
  if (typeof label === "object" && label.limit) return label.limit(wire);
  if (isISODate(wire)) {
    return formatISODate(
      wire,
      datePattern(locale, dateTimePreferences().dateFormat),
    );
  }
  const number = Number(wire);
  return wire.trim() !== "" && Number.isFinite(number)
    ? formatNumber(number, locale)
    : wire;
}

// A bound in minor units, shown as the amount the reader typed.
export function moneyLimit(
  currency: string,
  locale: Locale,
): (wire: string) => string {
  return (wire) => formatMoney(Number(wire), currency, locale);
}

// A bound in percent, shown with the reader's percent sign.
export function percentLimit(locale: Locale): (wire: string) => string {
  const format = numberFormat(locale, {
    style: "percent",
    maximumFractionDigits: 2,
  });
  return (wire) => format.format(Number(wire) / 100);
}
