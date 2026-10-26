// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { Button, Field, Radio } from "../../design-system/atoms";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import type { ListChip } from "../../design-system/listsurface";
import { MoneyInput } from "../../design-system/moneyinput";
import {
  currencyOptions,
  type LeadingCurrencies,
} from "../../format/currencychoices";
import { formatDate, formatMoney } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import {
  CLOSE_DAY_DIAL,
  encodeRange,
  parseDayRange,
  parseMinorRange,
  THIS_QUARTER,
  VALUE_DIAL,
} from "../../lib/dealranges";
import { countryOptions } from "./assignedcountries";

// The deal list's commercial dials: when a deal closes, what it is worth, what
// it is priced in, and where its company is. The two ranges are typed rather
// than picked, so each draws its own value step (`ListChip.editor`).

type Pick = (value: string, label: string) => void;

type RangeWords = Readonly<{
  t: Translator;
  locale: Locale;
  /** The zone the record's calendar days are shown in. */
  zone: string;
}>;

/** The words an applied close-day value reads as, or undefined for none. */
export function closeDayName(
  value: string | undefined,
  { t, locale, zone }: RangeWords,
): string | undefined {
  if (value === THIS_QUARTER) return t("reporting.this_quarter");
  const range = parseDayRange(value);
  if (!range) return undefined;
  const day = (iso: string) => formatDate(iso, locale, zone);
  return boundsName(
    range.from && day(range.from),
    range.to && day(range.to),
    t,
  );
}

/** The words an applied value range reads as, in the base currency. */
export function valueName(
  value: string | undefined,
  currency: string,
  { t, locale }: RangeWords,
): string | undefined {
  const range = parseMinorRange(value);
  if (!range) return undefined;
  const money = (minor: number) => formatMoney(minor, currency, locale);
  return boundsName(
    range.min === undefined ? undefined : money(range.min),
    range.max === undefined ? undefined : money(range.max),
    t,
  );
}

function boundsName(
  low: string | undefined,
  high: string | undefined,
  t: Translator,
): string | undefined {
  if (low && high) return `${low} – ${high}`;
  if (low) return t("deals.rangeFrom", { value: low });
  return high && t("deals.rangeTo", { value: high });
}

/** The value step's "any" entry, which is also how the dial is cleared. */
function AnyValue({
  group,
  value,
  label,
  onPick,
}: Readonly<{ group: string; value: string; label: string; onPick: Pick }>) {
  return (
    <Radio
      className="lt-mi"
      name={group}
      checked={!value}
      label={label}
      onChange={() => onPick("", label)}
    />
  );
}

function CloseDayEditor({
  value,
  onPick,
  anyLabel,
  words,
}: Readonly<{
  value: string;
  onPick: Pick;
  anyLabel: string;
  words: RangeWords;
}>) {
  const { t } = words;
  const group = useId();
  const typed = parseDayRange(value);
  const [from, setFrom] = useState(isoOrBlank(typed?.from));
  const [to, setTo] = useState(isoOrBlank(typed?.to));
  const draft = encodeRange(from, to);
  const quarter = t("reporting.this_quarter");
  return (
    <>
      <AnyValue group={group} value={value} label={anyLabel} onPick={onPick} />
      <Radio
        className="lt-mi"
        name={group}
        checked={value === THIS_QUARTER}
        label={quarter}
        onChange={() => onPick(THIS_QUARTER, quarter)}
      />
      <div className="lt-frange">
        <Field label={t("reporting.fromDate")}>
          {(field) => (
            <DateInput
              {...field}
              value={from}
              onChange={(event) => setFrom(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("reporting.throughDate")}>
          {(field) => (
            <DateInput
              {...field}
              value={to}
              onChange={(event) => setTo(event.target.value)}
            />
          )}
        </Field>
        <ApplyRange
          draft={draft}
          admitted={parseDayRange(draft) !== undefined}
          name={() => closeDayName(draft, words) ?? draft}
          onPick={onPick}
          t={t}
        />
      </div>
    </>
  );
}

function ValueRangeEditor({
  value,
  currency,
  onPick,
  anyLabel,
  words,
}: Readonly<{
  value: string;
  currency: string;
  onPick: Pick;
  anyLabel: string;
  words: RangeWords;
}>) {
  const { t } = words;
  const group = useId();
  const typed = parseMinorRange(value);
  const [min, setMin] = useState(typed?.min);
  const [max, setMax] = useState(typed?.max);
  const draft = encodeRange(
    min === undefined ? "" : String(min),
    max === undefined ? "" : String(max),
  );
  const bound = (
    label: string,
    minor: number | undefined,
    set: (next: number | undefined) => void,
  ) => (
    <MoneyInput
      aria-label={label}
      valueMinor={minor ?? 0}
      currency={currency}
      blankWhenZero={minor === undefined}
      onChangeMinor={(next) => set(Number.isNaN(next) ? undefined : next)}
      onClear={() => set(undefined)}
      inputMode="decimal"
    />
  );
  return (
    <>
      <AnyValue group={group} value={value} label={anyLabel} onPick={onPick} />
      <div className="lt-frange">
        {bound(t("deals.valueAtLeast", { currency }), min, setMin)}
        {bound(t("deals.valueAtMost", { currency }), max, setMax)}
        <ApplyRange
          draft={draft}
          admitted={parseMinorRange(draft) !== undefined}
          name={() => valueName(draft, currency, words) ?? draft}
          onPick={onPick}
          t={t}
        />
      </div>
    </>
  );
}

// Refused, with the reason under it, while the two ends say nothing or run
// backwards: the server refuses a backwards range, and nothing says nothing.
function ApplyRange({
  draft,
  admitted,
  name,
  onPick,
  t,
}: Readonly<{
  draft: string;
  admitted: boolean;
  name: () => string;
  onPick: Pick;
  t: Translator;
}>) {
  const reason = draft ? "deals.rangeBackwards" : "deals.rangeEmpty";
  return (
    <Button
      variant="primary"
      reason={admitted ? undefined : t(reason)}
      onClick={() => onPick(draft, name())}
    >
      {t("deals.rangeApply")}
    </Button>
  );
}

/**
 * The commercial dials as filter chips. Value is offered only once the base
 * currency is known, since its amounts are typed in that currency.
 */
export function dealRangeChips({
  words,
  filters,
  baseCurrency,
  leading,
}: Readonly<{
  words: RangeWords;
  filters: Readonly<Record<string, string>>;
  baseCurrency: string | undefined;
  leading: LeadingCurrencies;
}>): ListChip[] {
  const { t, locale } = words;
  const closeAny = t("deals.filterCloseDateAll");
  const chips: ListChip[] = [
    {
      key: CLOSE_DAY_DIAL,
      label: t("deals.filterCloseDate"),
      allLabel: closeAny,
      options: [],
      editor: (value, pick) => (
        <CloseDayEditor
          value={value}
          onPick={pick}
          anyLabel={closeAny}
          words={words}
        />
      ),
      appliedName: closeDayName(filters[CLOSE_DAY_DIAL], words),
    },
  ];
  if (baseCurrency) {
    const valueAny = t("deals.filterValueAll");
    chips.push({
      key: VALUE_DIAL,
      label: t("deals.filterValue"),
      allLabel: valueAny,
      options: [],
      editor: (value, pick) => (
        <ValueRangeEditor
          value={value}
          currency={baseCurrency}
          onPick={pick}
          anyLabel={valueAny}
          words={words}
        />
      ),
      appliedName: valueName(filters[VALUE_DIAL], baseCurrency, words),
    });
  }
  chips.push(
    {
      key: "currency",
      label: t("create.currency"),
      allLabel: t("deals.filterCurrencyAll"),
      options: currencyOptions(leading),
      filterable: true,
    },
    {
      key: "country",
      label: t("deals.filterCountry"),
      allLabel: t("deals.filterCountryAll"),
      options: countryOptions(locale),
      filterable: true,
    },
  );
  return chips;
}
