// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A number clause's operand, typed the reader's way and sent as a number.

import { useState } from "react";
import { ErrorLine } from "../../design-system/errorline";
import {
  typedNumberFault,
  typedNumberOrNaN,
  writtenDecimal,
} from "../../format/decimalinput";
import { type Locale, useLocale, useT } from "../../i18n";
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
