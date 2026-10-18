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

// A unit's spelling, lower-cased, to its key; each key spells itself. Rows
// written while the unit was free text ("Tag", "Stunde", "unit") show as their
// key without being rewritten. A declared mirror of UnitSpellings in
// backend/internal/modules/deals/unit.go, which stores a known spelling as its
// key; backend/gates/frontendunitspellings_test.go holds the two equal.
const UNIT_SPELLINGS: Readonly<Record<string, UnitKey>> = {
  piece: "piece",
  pieces: "piece",
  unit: "piece",
  units: "piece",
  pc: "piece",
  pcs: "piece",
  each: "piece",
  item: "piece",
  items: "piece",
  stück: "piece",
  stueck: "piece",
  stk: "piece",
  cái: "piece",
  hour: "hour",
  hours: "hour",
  h: "hour",
  hr: "hour",
  hrs: "hour",
  stunde: "hour",
  stunden: "hour",
  std: "hour",
  giờ: "hour",
  day: "day",
  days: "day",
  tag: "day",
  tage: "day",
  personentag: "day",
  personentage: "day",
  ngày: "day",
  month: "month",
  months: "month",
  monat: "month",
  monate: "month",
  tháng: "month",
  flat_fee: "flat_fee",
  "flat fee": "flat_fee",
  "flat rate": "flat_fee",
  "lump sum": "flat_fee",
  pauschal: "flat_fee",
  pauschale: "flat_fee",
  festpreis: "flat_fee",
  "trọn gói": "flat_fee",
};

function unitKeyOf(stored: string): UnitKey | undefined {
  const spelling = stored.trim().toLowerCase();
  return Object.hasOwn(UNIT_SPELLINGS, spelling)
    ? UNIT_SPELLINGS[spelling]
    : undefined;
}

// A stored unit as a reader sees it: a key in their language, typed text as typed.
export function unitLabel(stored: string, t: Translator): string {
  const key = unitKeyOf(stored);
  return key === undefined ? stored : t(UNIT_LABEL[key]);
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
  if (stored === "") {
    return { choice: "", other: "" };
  }
  const key = unitKeyOf(stored);
  return key === undefined
    ? { choice: OTHER_UNIT, other: stored }
    : { choice: key, other: "" };
}

// "Other" picked with nothing typed: a unit the reader started to name and did
// not, which a form refuses rather than sending no unit.
export function isUnnamedOtherUnit(value: UnitChoice): boolean {
  return value.choice === OTHER_UNIT && value.other.trim() === "";
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
