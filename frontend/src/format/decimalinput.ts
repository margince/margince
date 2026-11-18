// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale, Translator } from "../i18n";
import { INTL_LOCALE } from "./intllocale";

/** Why a typed number could not be read. */
export type DecimalRefusal = "ambiguousGrouping" | "grouping" | "notNumber";

export type ParsedDecimal =
  | Readonly<{ value: number }>
  | Readonly<{ refused: DecimalRefusal }>;

/**
 * A number as a reader types it, read the same in every browser and language.
 *
 * One comma or one dot is the decimal sign, so "12,5" and "12.5" are both
 * twelve and a half. A second separator, or a space inside the digits, is a
 * thousands separator, and it is refused rather than guessed.
 *
 * The locale decides only what is refused, never the value. Take its grouping
 * sign before three digits: "1.500" in German, "1,500" in English. That reader
 * means fifteen hundred and the rule above reads one and a half, so it is
 * refused. Exponents are refused too: nobody types `15e-1`.
 */
export function parseTypedDecimal(text: string, locale: Locale): ParsedDecimal {
  const trimmed = text.trim();
  if (/^[-+]?(\d+[.,]?\d*|[.,]\d+)$/.test(trimmed)) {
    const grouped = /^[-+]?\d+([.,])\d{3}$/.exec(trimmed);
    if (grouped && grouped[1] === groupingSign(locale)) {
      return { refused: "ambiguousGrouping" };
    }
    return { value: Number(trimmed.replace(",", ".")) };
  }
  if (/\d/.test(trimmed) && /^[-+]?[\d.,'\s  ]+$/.test(trimmed)) {
    return { refused: "grouping" };
  }
  return { refused: "notNumber" };
}

/**
 * The sign a locale groups thousands with. Read off a million, because some
 * locales (Spanish, Polish) leave a four-digit number ungrouped.
 */
export function groupingSign(locale: Locale): string {
  return signOf(locale, "group");
}

/**
 * The decimal sign a refusal message and a prefilled box write. Only a comma
 * or a dot, the two signs the parser reads.
 */
export function decimalSign(locale: Locale): "," | "." {
  return signOf(locale, "decimal") === "," ? "," : ".";
}

function signOf(locale: Locale, part: "group" | "decimal"): string {
  return (
    new Intl.NumberFormat(INTL_LOCALE[locale])
      .formatToParts(1_000_000.5)
      .find((candidate) => candidate.type === part)?.value ?? ""
  );
}

/** A plain decimal ("1234.5", from toFixed) written with the reader's sign. */
export function writtenDecimal(plain: string, locale: Locale): string {
  return plain.replace(".", decimalSign(locale));
}

/**
 * The two readings of an ambiguous "1.500": the whole number its grouping
 * means, and the fraction the decimal rule means, written the reader's way.
 */
export function ambiguousReadings(
  text: string,
  locale: Locale,
): Readonly<{ whole: string; fraction: string }> {
  const trimmed = text.trim().replace(/^\+/, "");
  const fraction = String(Number(trimmed.replace(/[.,]/, ".")));
  return {
    whole: trimmed.replace(/[.,]/, ""),
    fraction: writtenDecimal(fraction, locale),
  };
}

/**
 * The message under a box that takes a plain number (a quantity, a rate), or
 * undefined for a number or no text. Money boxes use typedAmountFault.
 */
export function typedNumberFault(
  text: string,
  locale: Locale,
  t: Translator,
): string | undefined {
  if (text.trim() === "") {
    return undefined;
  }
  const parsed = parseTypedDecimal(text, locale);
  if (!("refused" in parsed)) {
    return undefined;
  }
  if (parsed.refused === "ambiguousGrouping") {
    return t("number.refused.ambiguous", ambiguousReadings(text, locale));
  }
  const example = `1234${decimalSign(locale)}5`;
  return parsed.refused === "grouping"
    ? t("number.refused.grouping", { example })
    : t("number.refused.notNumber", { example });
}

/**
 * The number in a text a field's validate has already accepted, or NaN. For a
 * request mapper, as typedMinorOrNaN is for money.
 */
export function typedNumberOrNaN(text: string, locale: Locale): number {
  const parsed = parseTypedDecimal(text, locale);
  return "value" in parsed ? parsed.value : Number.NaN;
}

/**
 * An accepted text as the plain decimal a server reads ("12,5" is "12.5"), or
 * undefined when refused. Kept as text: a rate's digits survive, and no tiny
 * rate turns into an exponent the way String(number) writes it.
 */
export function typedDecimalText(
  text: string,
  locale: Locale,
): string | undefined {
  if ("refused" in parseTypedDecimal(text, locale)) {
    return undefined;
  }
  const plain = text.trim().replace(/^\+/, "").replace(",", ".");
  const [whole, fraction = ""] = plain.split(".");
  const integer = whole === "" || whole === "-" ? `${whole}0` : whole;
  return fraction === "" ? integer : `${integer}.${fraction}`;
}
