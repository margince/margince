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
