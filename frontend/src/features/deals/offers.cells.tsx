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
import type { OfferLineItem, UpdateOfferLineItemRequest } from "./offers";
import { storedUnitOf, UnitPicker, unitChoiceOf } from "./productunit";
import { useSavedDraft } from "./usesaveddraft";

// A save that answers when its request settles, so a cell can tell an answer
// from the draft it carried. A refused save rejects; its reason shows in the
// line's row, and the cell keeps the refused text for the reader to fix.
type LineSave<Value> = (value: Value) => Promise<unknown>;

export function DescriptionCell({
  line,
  label,
  onSave,
}: Readonly<{
  line: OfferLineItem;
  label: string;
  onSave: LineSave<UpdateOfferLineItemRequest>;
}>) {
  const { draft, edit, leave, unsaved } = useSavedDraft({
    saved: line.description,
    toDraft: (saved) => saved,
    submit: (text) => {
      return text === line.description
        ? undefined
        : onSave({ description: text });
    },
  });
  return (
    <TextInput
      aria-label={label}
      data-testid={`line-description-${line.id}`}
      className="offers-line-description"
      value={draft}
      aria-invalid={unsaved ? true : undefined}
      onChange={(event) => edit(event.target.value)}
      onBlur={() => leave()}
    />
  );
}

export function UnitCell({
  line,
  label,
  onSave,
}: Readonly<{
  line: OfferLineItem;
  label: (column: string) => string;
  onSave: LineSave<UpdateOfferLineItemRequest>;
}>) {
  const t = useT();
  const { draft, edit, leave } = useSavedDraft({
    saved: line.unit,
    toDraft: unitChoiceOf,
    submit: (next) => {
      // An "Other" with nothing typed yet names no unit, and the server
      // refuses a blank one.
      const unit = storedUnitOf(next.choice, next.other);
      return unit !== "" && unit !== line.unit ? onSave({ unit }) : undefined;
    },
  });
  return (
    <UnitPicker
      label={label(t("offer.unit"))}
      otherLabel={label(t("product.unitOtherLabel"))}
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
  label,
  onSave,
  check,
}: Readonly<{
  value: number;
  width: number;
  testId: string;
  label: string;
  onSave: LineSave<number>;
  // The field's own refusal of a typed text, bounds included. Absent, any
  // number passes.
  check?: (text: string) => string | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [fault, setFault] = useState<string | undefined>();
  const errorId = useId();
  const { draft, edit, leave, restore, unsaved } = useSavedDraft({
    saved: savedValue,
    toDraft: (saved) => writtenDecimal(String(saved), locale),
    submit: (text) => {
      const refusal = check ? check(text) : typedNumberFault(text, locale, t);
      if (refusal) {
        setFault(refusal);
        return;
      }
      const num = typedNumberOrNaN(text, locale);
      return num === savedValue ? undefined : onSave(num);
    },
  });
  return (
    <>
      <input
        aria-label={label}
        type="text"
        inputMode="decimal"
        className="input"
        style={{ width }}
        data-testid={testId}
        value={draft}
        aria-invalid={fault || unsaved ? true : undefined}
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
  label,
  onSave,
}: Readonly<{
  line: OfferLineItem;
  currency: string;
  label: string;
  onSave: LineSave<UpdateOfferLineItemRequest>;
}>) {
  const { draft, edit, leave, unsaved } = useSavedDraft({
    saved: line.unit_price_minor,
    toDraft: (saved) => saved,
    submit: (minor) => {
      return minor === line.unit_price_minor
        ? undefined
        : onSave({ unit_price_minor: minor });
    },
  });
  return (
    <MoneyInput
      aria-label={label}
      data-testid={`line-unit-price-${line.id}`}
      className="offers-line-price"
      currency={currency}
      valueMinor={draft}
      aria-invalid={unsaved ? true : undefined}
      onChangeMinor={edit}
      onBlur={() => leave()}
    />
  );
}
