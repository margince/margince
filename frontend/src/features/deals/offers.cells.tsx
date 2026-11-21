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

export function DescriptionCell({
  line,
  label,
  onSave,
}: Readonly<{
  line: OfferLineItem;
  label: string;
  onSave: (patch: UpdateOfferLineItemRequest) => void;
}>) {
  const [value, setValue] = useState(line.description);
  return (
    <TextInput
      aria-label={label}
      data-testid={`line-description-${line.id}`}
      className="offers-line-description"
      value={value}
      onChange={(event) => setValue(event.target.value)}
      onBlur={() => {
        if (value !== line.description) {
          onSave({ description: value });
        }
      }}
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
  onSave: (patch: UpdateOfferLineItemRequest) => void;
}>) {
  const t = useT();
  const [value, setValue] = useState(() => unitChoiceOf(line.unit));
  return (
    <UnitPicker
      label={label(t("offer.unit"))}
      otherLabel={label(t("product.unitOtherLabel"))}
      testId={`line-unit-${line.id}`}
      value={value}
      onChange={(next, settled) => {
        setValue(next);
        // An "Other" with nothing typed yet names no unit, and the server
        // refuses a blank one.
        const unit = storedUnitOf(next.choice, next.other);
        if (settled && unit !== "" && unit !== line.unit) {
          onSave({ unit });
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
  onSave: (num: number) => void;
  // The field's own refusal of a typed text, bounds included. Absent, any
  // number passes.
  check?: (text: string) => string | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [value, setValue] = useState(() =>
    writtenDecimal(String(savedValue), locale),
  );
  const [fault, setFault] = useState<string | undefined>();
  const errorId = useId();
  return (
    <>
      <input
        aria-label={label}
        type="text"
        inputMode="decimal"
        className="input"
        style={{ width }}
        data-testid={testId}
        value={value}
        aria-invalid={fault ? true : undefined}
        aria-describedby={fault ? errorId : undefined}
        onChange={(event) => {
          setValue(event.target.value);
          setFault(undefined);
        }}
        onBlur={() => {
          if (value.trim() === "") {
            setValue(writtenDecimal(String(savedValue), locale));
            return;
          }
          const refusal = check
            ? check(value)
            : typedNumberFault(value, locale, t);
          if (refusal) {
            setFault(refusal);
            return;
          }
          const num = typedNumberOrNaN(value, locale);
          if (num !== savedValue) {
            onSave(num);
          }
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
  onSave: (patch: UpdateOfferLineItemRequest) => void;
}>) {
  const [minor, setMinor] = useState(line.unit_price_minor);
  return (
    <MoneyInput
      aria-label={label}
      data-testid={`line-unit-price-${line.id}`}
      className="offers-line-price"
      currency={currency}
      valueMinor={minor}
      onChangeMinor={setMinor}
      onBlur={() => {
        if (minor !== line.unit_price_minor) {
          onSave({ unit_price_minor: minor });
        }
      }}
    />
  );
}
