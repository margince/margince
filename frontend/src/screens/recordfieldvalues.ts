import { formatMoney } from "../format/format";
import { toMinorUnits } from "../format/minorunits";
import type { Locale, useT } from "../i18n";
import {
  type CreateField,
  type FormRows,
  fieldLabel,
  fieldOptions,
  splitMultiselectValue,
} from "./create";
import { prefillFromRecord, prefillRowsFromRecord } from "./edit.prefill";

export function selectOptions(
  field: CreateField,
  values: Record<string, string>,
  t: ReturnType<typeof useT>,
) {
  const options = fieldOptions(field, values);
  return field.required || options.some((option) => option.value === "")
    ? options
    : [{ value: "", label: t("field.unset") }, ...options];
}

function optionLabel(
  field: CreateField,
  value: string,
  values: Record<string, string>,
  t: ReturnType<typeof useT>,
) {
  return (
    selectOptions(field, values, t).find((option) => option.value === value)
      ?.label ?? value
  );
}

export function groupValue(
  fields: CreateField[],
  record: Record<string, unknown>,
  t: ReturnType<typeof useT>,
  locale: Locale,
  maskedFields: readonly string[] = [],
  readOnlyFields: Readonly<Record<string, string>> = {},
): string {
  const values = prefillFromRecord(fields, record);
  const code = values.currency ?? "";
  // A group that pairs figures with a currency code reads as money. The code
  // is inside each figure, so it is not repeated as its own part.
  const moneyLocale = /^[A-Z]{3}$/.test(code) ? locale : undefined;
  const rows = prefillRowsFromRecord(fields, record);
  if (
    isAddressGroup(fields) &&
    !fields.some((f) => maskedFields.includes(f.key))
  ) {
    return postalLines(values);
  }
  return fields
    .map((field) => {
      if (field.searchTargets)
        return optionLabel(field, values[field.key] ?? "", values, t);
      const value = values[field.key] ?? "";
      const money = moneyPart(
        field,
        value,
        code,
        moneyLocale,
        fields.length,
        t,
      );
      if (money !== undefined) return money;
      return plainPart(field, value, {
        values,
        rows,
        readOnlyFields,
        partCount: fields.length,
        t,
      });
    })
    .map((value, index) =>
      maskedFields.includes(fields[index].key)
        ? `${fieldLabel(fields[index], t)}: ${t("record.notShown")}`
        : value,
    )
    .filter(Boolean)
    .join(" · ");
}

function plainPart(
  field: CreateField,
  value: string,
  ctx: {
    values: Record<string, string>;
    rows: FormRows;
    readOnlyFields: Readonly<Record<string, string>>;
    partCount: number;
    t: ReturnType<typeof useT>;
  },
): string {
  const { values, rows, readOnlyFields, partCount, t } = ctx;
  switch (field.type) {
    case "repeatable":
      return repeatableValue(field, rows);
    case "multiselect":
      return splitMultiselectValue(value, field.multiselectEncoding)
        .map((item) => optionLabel(field, item, values, t))
        .join(", ");
    case "select":
      return value || field.key in readOnlyFields
        ? optionLabel(field, value, values, t)
        : "";
    case "number":
      return value && partCount > 1
        ? `${fieldLabel(field, t)}: ${value}`
        : value;
  }
  return value;
}

// One part of a money group. A figure is written in its currency, the
// currency field itself is empty, and a part that is not money is undefined.
function moneyPart(
  field: CreateField,
  value: string,
  code: string,
  locale: Locale | undefined,
  partCount: number,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (!locale) return undefined;
  if (field.key === "currency") return "";
  if (field.type !== "number" || !value) return undefined;
  const shown = formatMoney(toMinorUnits(Number(value), code), code, locale);
  return partCount > 2 ? `${fieldLabel(field, t)}: ${shown}` : shown;
}

function repeatableValue(field: CreateField, rows: FormRows): string {
  const key = field.rowFields?.[0]?.key ?? "";
  return (rows[field.key] ?? [])
    .map((row) => row[key])
    .filter(Boolean)
    .join(", ");
}

// An address group is every field of the six the record's address is split
// into, and nothing else: the one group whose parts have a shape of their own.
function isAddressGroup(fields: CreateField[]): boolean {
  return (
    fields.length > 0 &&
    fields.every((field) => field.key.startsWith("address_"))
  );
}

// The address the way a reader writes it on an envelope: street lines, then
// the postal code before the city, then region and country. One value per
// line rather than six clauses on dots, which read as a list of unrelated
// facts. The lines are joined on a newline the value column keeps
// (`white-space: pre-line`, fieldgrid.css).
function postalLines(values: Record<string, string>): string {
  const place = [values.address_postal_code, values.address_city]
    .filter(Boolean)
    .join(" ");
  const area = [values.address_region, values.address_country]
    .filter(Boolean)
    .join(", ");
  return [values.address_line1, values.address_line2, place, area]
    .filter(Boolean)
    .join("\n");
}
