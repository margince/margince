// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { INTL_LOCALE } from "../format/intllocale";
import { useLocale, useT } from "../i18n";
import { type FieldControl, TextInput } from "./field";
import { Select } from "./select";
import "./monthinput.css";

/** A calendar month as `YYYY-MM`. */
export type YearMonth = `${number}-${number}`;

export function isYearMonth(raw: string): raw is YearMonth {
  const match = /^(\d{4})-(\d{2})$/.exec(raw);
  return match !== null && Number(match[2]) >= 1 && Number(match[2]) <= 12;
}

/**
 * The one month field: the month by name in the UI language, and its year.
 * A native `month` input draws both in the browser's language instead.
 * `onChange` hears `YYYY-MM` once both are set, and `""` once the month is cleared.
 */
export function MonthInput({
  value,
  onChange,
  disabled,
  ...control
}: Partial<FieldControl> &
  Readonly<{
    value: YearMonth | "";
    onChange: (next: YearMonth | "") => void;
    disabled?: boolean;
  }>) {
  const t = useT();
  const { locale } = useLocale();
  const [year, setYear] = useState(value.slice(0, 4));
  const month = value.slice(5, 7);
  const names = new Intl.DateTimeFormat(INTL_LOCALE[locale], { month: "long" });
  const options = [
    { value: "", label: t("calendar.anyMonth") },
    ...Array.from({ length: 12 }, (_, index) => ({
      value: String(index + 1).padStart(2, "0"),
      label: names.format(new Date(2000, index, 1)),
    })),
  ];
  const report = (nextMonth: string, nextYear: string) => {
    const next = `${nextYear}-${nextMonth}`;
    onChange(isYearMonth(next) ? next : "");
  };

  return (
    <span className="month-input">
      <Select
        {...control}
        disabled={disabled}
        options={options}
        value={month}
        onChange={(nextMonth) => {
          const nextYear =
            year === "" ? String(new Date().getFullYear()) : year;
          setYear(nextYear);
          report(nextMonth, nextYear);
        }}
      />
      <TextInput
        aria-label={t("calendar.year")}
        inputMode="numeric"
        maxLength={4}
        disabled={disabled}
        className="month-input-year"
        placeholder={t("calendar.patternYear")}
        value={year}
        onChange={(event) => {
          const nextYear = event.target.value.replace(/\D/g, "");
          setYear(nextYear);
          if (month !== "" && nextYear.length === 4) {
            report(month, nextYear);
          }
        }}
      />
    </span>
  );
}
