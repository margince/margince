import { useId, useState } from "react";
import { TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { MoneyInput } from "../../design-system/moneyinput";
import {
  typedNumberFault,
  typedNumberOrNaN,
  writtenDecimal,
} from "../../format/decimalinput";
import { useLocale, useT } from "../../i18n";
import { useSavedDraft } from "../../lib/usesaveddraft";
import type { OfferLineItem, UpdateOfferLineItemRequest } from "./offers";
import { storedUnitOf, UnitPicker, unitChoiceOf } from "./productunit";

export function DescriptionCell({
  line,
  onSave,
}: Readonly<{
  line: OfferLineItem;
  onSave: (patch: UpdateOfferLineItemRequest) => void;
}>) {
  const { draft, edit, leave } = useSavedDraft({
    saved: line.description,
    toDraft: (saved) => saved,
    submit: (text) => {
      if (text !== line.description) {
        onSave({ description: text });
      }
    },
  });
  return (
    <TextInput
      data-testid={`line-description-${line.id}`}
      className="offers-line-description"
      value={draft}
      onChange={(event) => edit(event.target.value)}
      onBlur={() => leave()}
    />
  );
}

export function UnitCell({
  line,
  onSave,
}: Readonly<{
  line: OfferLineItem;
  onSave: (patch: UpdateOfferLineItemRequest) => void;
}>) {
  const { draft, edit, leave } = useSavedDraft({
    saved: line.unit,
    toDraft: unitChoiceOf,
    submit: (next) => {
      // An "Other" with nothing typed yet names no unit, and the server
      // refuses a blank one.
      const unit = storedUnitOf(next.choice, next.other);
      if (unit !== "" && unit !== line.unit) {
        onSave({ unit });
      }
    },
  });
  return (
    <UnitPicker
      testId={`line-unit-${line.id}`}
      value={draft}
      onChange={(next, settled) => {
        edit(next);
        if (settled) {
          leave(next);
        }
      }}
    />
  );
}

// The shared numeric-line-field editor: position, quantity, discount and tax
// rate are all a local text buffer that commits a parsed number on blur.
// Blurring an emptied field restores the last saved value, so clearing a field
// to retype it never zeroes it. A refused text stays, with the reason below.
export function NumericCell({
  value: savedValue,
  width,
  testId,
  onSave,
  check,
}: Readonly<{
  value: number;
  width: number;
  testId: string;
  onSave: (num: number) => void;
  // The field's own refusal of a typed text, bounds included. Absent, any
  // number passes.
  check?: (text: string) => string | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [fault, setFault] = useState<string | undefined>();
  const errorId = useId();
  const { draft, edit, leave, restore } = useSavedDraft({
    saved: savedValue,
    toDraft: (saved) => writtenDecimal(String(saved), locale),
    submit: (text) => {
      const refusal = check ? check(text) : typedNumberFault(text, locale, t);
      if (refusal) {
        setFault(refusal);
        return;
      }
      const num = typedNumberOrNaN(text, locale);
      if (num !== savedValue) {
        onSave(num);
      }
    },
  });
  return (
    <>
      <input
        type="text"
        inputMode="decimal"
        className="input"
        style={{ width }}
        data-testid={testId}
        value={draft}
        aria-invalid={fault ? true : undefined}
        aria-describedby={fault ? errorId : undefined}
        onChange={(event) => {
          edit(event.target.value);
          setFault(undefined);
        }}
        onBlur={() => {
          if (draft.trim() === "") {
            restore();
            return;
          }
          leave();
        }}
      />
      {fault && <ErrorLine id={errorId}>{fault}</ErrorLine>}
    </>
  );
}

export function UnitPriceCell({
  line,
  currency,
  onSave,
}: Readonly<{
  line: OfferLineItem;
  currency: string;
  onSave: (patch: UpdateOfferLineItemRequest) => void;
}>) {
  const { draft, edit, leave } = useSavedDraft({
    saved: line.unit_price_minor,
    toDraft: (saved) => saved,
    submit: (minor) => {
      if (minor !== line.unit_price_minor) {
        onSave({ unit_price_minor: minor });
      }
    },
  });
  return (
    <MoneyInput
      data-testid={`line-unit-price-${line.id}`}
      className="offers-line-price"
      currency={currency}
      valueMinor={draft}
      onChangeMinor={edit}
      onBlur={() => leave()}
    />
  );
}
