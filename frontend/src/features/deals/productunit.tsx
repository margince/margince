// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { TextInput } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { type Translator, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import "./offers.css";

// The units a product or an offer line is priced in. The wire carries the key,
// so a reader sees the unit in their own language. Any other stored text is a
// unit somebody typed, shown as written. "piece" leads because it is the
// server's default, and a form preselects the first choice.
const UNIT_KEYS = ["piece", "hour", "day", "month", "flat_fee"] as const;

type UnitKey = (typeof UNIT_KEYS)[number];

export const OTHER_UNIT = "other";

const UNIT_LABEL: Readonly<Record<UnitKey, MessageKey>> = {
  piece: "product.unitPiece",
  hour: "product.unitHour",
  day: "product.unitDay",
  month: "product.unitMonth",
  flat_fee: "product.unitFlatFee",
};

function isUnitKey(unit: string): unit is UnitKey {
  return (UNIT_KEYS as readonly string[]).includes(unit);
}

// A stored unit as a reader sees it: a key in their language, typed text as typed.
export function unitLabel(stored: string, t: Translator): string {
  return isUnitKey(stored) ? t(UNIT_LABEL[stored]) : stored;
}

// The choices a unit picker offers: the keys in words, then "Other".
export function unitOptions(t: Translator): { value: string; label: string }[] {
  return [
    ...UNIT_KEYS.map((key) => ({ value: key, label: t(UNIT_LABEL[key]) })),
    { value: OTHER_UNIT, label: t("product.unitOther") },
  ];
}

// A stored unit split into the picker's two controls. An empty unit stays
// unchosen, so a form that leaves it alone sends nothing.
export function unitChoiceOf(stored: string): {
  choice: string;
  other: string;
} {
  if (stored === "" || isUnitKey(stored)) {
    return { choice: stored, other: "" };
  }
  return { choice: OTHER_UNIT, other: stored };
}

// The picker's two controls joined into what the wire stores: the key, or the
// typed unit when "Other" is picked. Empty means the reader named no unit.
export function storedUnitOf(choice: string, other: string): string {
  return choice === OTHER_UNIT ? other.trim() : choice;
}

export type UnitChoice = Readonly<{ choice: string; other: string }>;

/**
 * A unit as a picklist, with a text box for a unit the list does not have.
 *
 * `settled` says the reader is done, for a caller that saves as it goes. A
 * pick is done at once. A typed unit is done when its box loses focus.
 */
export function UnitPicker({
  value,
  onChange,
  blankLabel,
  testId,
  id,
}: Readonly<{
  value: UnitChoice;
  onChange: (next: UnitChoice, settled: boolean) => void;
  // Offers an empty first choice under this label, for a form where naming no
  // unit means something.
  blankLabel?: string;
  testId: string;
  // A surrounding Field's control id, whose label then names the list.
  id?: string;
}>) {
  const t = useT();
  const options = unitOptions(t);
  return (
    <>
      <Select
        id={id}
        aria-label={id === undefined ? t("offer.unit") : undefined}
        testId={testId}
        className="offers-line-unit"
        value={value.choice}
        onChange={(choice) => onChange({ ...value, choice }, true)}
        options={
          blankLabel === undefined
            ? options
            : [{ value: "", label: blankLabel }, ...options]
        }
      />
      {value.choice === OTHER_UNIT && (
        <TextInput
          aria-label={t("product.unitOtherLabel")}
          data-testid={`${testId}-other`}
          className="offers-line-unit"
          value={value.other}
          onChange={(event) =>
            onChange({ ...value, other: event.target.value }, false)
          }
          onBlur={() => onChange(value, true)}
        />
      )}
    </>
  );
}
