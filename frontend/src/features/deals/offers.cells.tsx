import { useState } from "react";
import { TextInput } from "../../design-system/atoms";
import { MoneyInput } from "../../design-system/moneyinput";
import type { OfferLineItem, UpdateOfferLineItemRequest } from "./offers";

export function DescriptionCell({
  line,
  onSave,
}: Readonly<{
  line: OfferLineItem;
  onSave: (patch: UpdateOfferLineItemRequest) => void;
}>) {
  const [value, setValue] = useState(line.description);
  return (
    <TextInput
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
  onSave,
}: Readonly<{
  line: OfferLineItem;
  onSave: (patch: UpdateOfferLineItemRequest) => void;
}>) {
  const [value, setValue] = useState(line.unit);
  return (
    <TextInput
      data-testid={`line-unit-${line.id}`}
      className="offers-line-unit"
      value={value}
      onChange={(event) => setValue(event.target.value)}
      onBlur={() => {
        if (value !== line.unit) {
          onSave({ unit: value });
        }
      }}
    />
  );
}

// The shared numeric-line-field editor: position, quantity, discount and tax
// rate are all a local text buffer that commits a parsed number on blur.
// Blurring an emptied field restores the last saved value. Number("") is 0,
// not NaN, so without this guard, clearing a field to retype it would zero it
// the moment focus left the input.
export function NumericCell({
  value: savedValue,
  step,
  width,
  testId,
  onSave,
}: Readonly<{
  value: number;
  step: string;
  width: number;
  testId: string;
  onSave: (num: number) => void;
}>) {
  const [value, setValue] = useState(String(savedValue));
  return (
    <input
      type="number"
      step={step}
      className="input"
      style={{ width }}
      data-testid={testId}
      value={value}
      onChange={(event) => setValue(event.target.value)}
      onBlur={() => {
        if (value.trim() === "") {
          setValue(String(savedValue));
          return;
        }
        const num = Number(value);
        if (!Number.isNaN(num) && num !== savedValue) {
          onSave(num);
        }
      }}
    />
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
  const [minor, setMinor] = useState(line.unit_price_minor);
  return (
    <MoneyInput
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
