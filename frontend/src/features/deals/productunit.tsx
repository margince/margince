// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { TextInput } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import {
  OTHER_UNIT,
  type UnitChoice,
  unitOptions,
} from "../../format/productunit";
import { useT } from "../../i18n";

export {
  isUnnamedOtherUnit,
  OTHER_UNIT,
  storedUnitOf,
  type UnitChoice,
  unitChoiceOf,
  unitLabel,
  unitOptions,
} from "../../format/productunit";

import "./offers.css";

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
