import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Button, Field } from "../design-system/atoms";
import type { ListChip, ListColumn } from "../design-system/listtable";
import { useLocale, useT } from "../i18n";
import type { CfObject } from "../lib/customfields.logic";
import { customFieldsQuery } from "../lib/customfields.query";
import { isRecord } from "../lib/problembody";
import { useRecordKindDirectory } from "../lib/recordkinds";
import { fieldControl } from "./create.controls";
import type { CreateField } from "./create.fields";
import {
  type CustomField,
  customFieldFormValue,
  customFieldsToBody,
  customFieldToFormField,
  fieldsForSurface,
  useCustomFieldDisplay,
  useCustomFieldLabel,
} from "./customfields.form";
import { renderCustomFieldValue } from "./recordcustomfields";

export function useListCustomFields<Row>(
  object: CfObject | undefined,
  filters: Readonly<Record<string, string>>,
) {
  const query = useQuery(customFieldsQuery(object));
  const fields = fieldsForSurface(query.data?.data ?? [], "record");
  const read = useCustomFieldDisplay();
  const label = useCustomFieldLabel();
  const t = useT();
  const { locale } = useLocale();
  const kinds = useRecordKindDirectory(
    fields.some((field) => field.type === "record"),
  ).data?.data;
  const boolLabels = { yes: t("field.yes"), no: t("field.no") };
  const columns: ListColumn<Row>[] = fields.map((field) => ({
    key: field.column_name,
    header: label(field),
    initiallyHidden: true,
    sort: field.type === "multiselect" ? undefined : field.column_name,
    numeric: field.type === "number" || field.type === "currency",
    cell: (row) => {
      if (!isRecord(row)) {
        return null;
      }
      if (
        Array.isArray(row.masked_fields) &&
        row.masked_fields.includes(field.column_name)
      ) {
        return t("record.notShown");
      }
      const display = read(field, row[field.column_name]);
      return display ? renderCustomFieldValue(field, display) : null;
    },
  }));
  const chips: ListChip[] = fields.map((field) => {
    const control = customFieldToFormField(field, boolLabels, t, locale, kinds);
    const current = filters[field.column_name];
    return {
      key: field.column_name,
      label: label(field),
      allLabel: t("cf.filterAny"),
      options: [],
      appliedName:
        current === undefined
          ? undefined
          : (read(field, filterValue(field, current)) ?? current),
      editor: (value, pick) => (
        <CustomFieldFilter
          key={field.id}
          field={field}
          control={control}
          value={value}
          pick={pick}
        />
      ),
    };
  });
  return { columns, chips };
}

function filterValue(field: CustomField, value: string): unknown {
  if (field.type === "boolean") {
    return value === "true";
  }
  if (field.type === "multiselect") {
    try {
      const parsed: unknown = JSON.parse(value);
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  }
  return value;
}

function CustomFieldFilter({
  field,
  control,
  value,
  pick,
}: Readonly<{
  field: CustomField;
  control: CreateField;
  value: string;
  pick: (value: string, label: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const read = useCustomFieldDisplay();
  const label = useCustomFieldLabel();
  const [draft, setDraft] = useState(() =>
    customFieldFormValue(field, filterValue(field, value), locale),
  );
  const fault = control.validate?.(draft);
  const stored = customFieldsToBody(
    { [field.column_name]: draft },
    [field],
    locale,
  )[field.column_name];
  const wire =
    stored == null
      ? ""
      : Array.isArray(stored)
        ? JSON.stringify(stored)
        : String(stored);
  return (
    <div className="lt-frange">
      <Field label={label(field)}>
        {(binding) => fieldControl(control, binding, draft, setDraft, t)}
      </Field>
      <Button
        variant="primary"
        reason={fault ?? (!wire ? t("cf.filterValueRequired") : undefined)}
        onClick={() => pick(wire, read(field, stored) ?? draft)}
      >
        {t("cf.filterApply")}
      </Button>
    </div>
  );
}
