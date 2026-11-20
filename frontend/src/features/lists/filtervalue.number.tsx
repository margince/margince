// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A number clause's operand, typed the reader's way and sent as a number, and
// the list `in` takes, whose numbers are read the same way.

import { useState } from "react";
import { ErrorLine } from "../../design-system/errorline";
import { TokenInput, type TokenParse } from "../../design-system/tokeninput";
import {
  parseTypedDecimal,
  typedNumberFault,
  typedNumberOrNaN,
  writtenDecimal,
} from "../../format/decimalinput";
import { type Locale, useLocale, useT } from "../../i18n";
import type { VocabularyField } from "../../lib/filterdata";
import type { LeafValue } from "../../lib/segmentpredicate";

/**
 * The text is held here as typed. A half-typed "-" or "1," is not yet a number,
 * and a refused "1.500" stays in view with the reason under it. The tree holds
 * the typed text until it reads, so `isComplete` keeps the preview back.
 */
export function NumberValue({
  value,
  onChange,
  label,
}: Readonly<{
  value: LeafValue;
  onChange: (next: LeafValue) => void;
  label: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [text, setText] = useState(() => textOf(value, locale));
  // A change the tree made without this box, such as Discard, replaces it.
  const [seen, setSeen] = useState<LeafValue>(value);
  if (seen !== value) {
    setSeen(value);
    if (value !== operandOf(text, locale)) {
      setText(textOf(value, locale));
    }
  }
  const fault = typedNumberFault(text, locale, t);
  return (
    <>
      <input
        className="input"
        value={text}
        onChange={(event) => {
          setText(event.target.value);
          onChange(operandOf(event.target.value, locale));
        }}
        aria-label={label}
        aria-invalid={fault ? true : undefined}
        inputMode="decimal"
      />
      {fault && <ErrorLine>{fault}</ErrorLine>}
    </>
  );
}

/** An `in` operand: numbers on a number field, words on any other. */
export function ListValue({
  type,
  value,
  onChange,
  label,
}: Readonly<{
  type: VocabularyField["type"];
  value: LeafValue;
  onChange: (next: LeafValue) => void;
  label: string;
}>) {
  const t = useT();
  if (type === "number") {
    return <NumberListValue value={value} onChange={onChange} label={label} />;
  }
  return (
    <TokenInput
      values={Array.isArray(value) ? value.map(String) : []}
      onChange={(next) => onChange(next)}
      removeLabel={(item) => t("tokenInput.remove", { value: item })}
      aria-label={label}
      placeholder={t("filters.addValue")}
    />
  );
}

/**
 * A number clause's `in` list, read token by token as the single box reads its
 * text. A semicolon parts the numbers, since "12,5" is one number in German.
 */
function NumberListValue({
  value,
  onChange,
  label,
}: Readonly<{
  value: LeafValue;
  onChange: (next: LeafValue) => void;
  label: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The first part the box refused, named under it until none is left.
  const [refused, setRefused] = useState<string | undefined>();
  const unread = (part: string) => "refused" in parseTypedDecimal(part, locale);
  const parse: TokenParse = {
    keys: [";"],
    read: (raw) => {
      const parts = raw.split(";").filter((part) => part.trim() !== "");
      return {
        values: parts.filter((part) => !unread(part)),
        refused: parts.filter(unread),
      };
    },
    onRefuse: (parts) => setRefused(parts[0]),
  };
  const fault = refused && typedNumberFault(refused, locale, t);
  return (
    <>
      <TokenInput
        values={
          Array.isArray(value) ? value.map((item) => textOf(item, locale)) : []
        }
        onChange={(next) => onChange(numbersOrText(next, locale))}
        parse={parse}
        removeLabel={(item) => t("tokenInput.remove", { value: item })}
        aria-label={label}
        aria-invalid={fault ? true : undefined}
        placeholder={t("filters.addValue")}
      />
      {fault && <ErrorLine>{fault}</ErrorLine>}
    </>
  );
}

/**
 * The listed numbers, or the text as held while any item is not one. The
 * engine then refuses a restored list by name rather than this screen dropping
 * an item.
 */
function numbersOrText(typed: readonly string[], locale: Locale): LeafValue {
  const numbers = typed.map((text) => typedNumberOrNaN(text, locale));
  return numbers.some(Number.isNaN) ? typed : numbers;
}

function textOf(value: LeafValue, locale: Locale): string {
  if (typeof value === "number") {
    return writtenDecimal(String(value), locale);
  }
  return typeof value === "string" ? value : "";
}

/** A number when the text reads as one, and the text as typed otherwise. */
function operandOf(text: string, locale: Locale): LeafValue {
  if (text.trim() === "") {
    return "";
  }
  const number = typedNumberOrNaN(text, locale);
  return Number.isNaN(number) ? text : number;
}
