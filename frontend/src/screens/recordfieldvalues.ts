import type { useT } from "../i18n";
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
  maskedFields: readonly string[] = [],
  readOnlyFields: Readonly<Record<string, string>> = {},
): string {
  const values = prefillFromRecord(fields, record);
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
      switch (field.type) {
        case "repeatable":
          return repeatableValue(field, rows);
        case "multiselect":
          return splitMultiselectValue(value)
            .map((item) => optionLabel(field, item, values, t))
            .join(", ");
        case "select":
          return value || field.key in readOnlyFields
            ? optionLabel(field, value, values, t)
            : "";
        case "number":
          return value && fields.length > 1
            ? `${fieldLabel(field, t)}: ${value}`
            : value;
      }
      return value;
    })
    .map((value, index) =>
      maskedFields.includes(fields[index].key)
        ? `${fieldLabel(fields[index], t)}: ${t("record.notShown")}`
        : value,
    )
    .filter(Boolean)
    .join(" · ");
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
