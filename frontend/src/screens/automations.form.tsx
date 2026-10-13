import { type ReactNode, useId, useState } from "react";
import type { components } from "../api/schema";
import { Button, Checkbox, Field, TextInput } from "../design-system/atoms";
import { RecordFormDialog } from "../design-system/recordformdialog";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { DateFieldSelect } from "./automations.datefield";
import { ListParamSelect } from "./automations.lists";
import {
  type ParamField,
  paramFields,
  paramsFromValues,
  scalarText,
} from "./automations.params";
import { recipeSentence } from "./automations.recipe";

type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];
type Automation = components["schemas"]["Automation"];

// The dialog an automation is named and parameterised through, and the one
// control that draws a single declared parameter. Create in the admin panel and
// edit on a row both open it.

function ParamFieldControl({
  field,
  value,
  object,
  watchedList,
  onChange,
}: Readonly<{
  field: ParamField;
  value: string;
  object: string;
  /** The Live List a list rule watches, which narrows its Shortlist picker. */
  watchedList: string;
  onChange: (value: string) => void;
}>) {
  if (field.kind === "boolean") {
    return (
      <div className="field">
        <Checkbox
          label={field.key}
          checked={value === "true"}
          onChange={(event) =>
            onChange(event.target.checked ? "true" : "false")
          }
        />
      </div>
    );
  }
  if (field.kind === "live_list" || field.kind === "shortlist") {
    const kind = field.kind;
    return (
      <Field label={field.key}>
        {(control) => (
          <ListParamSelect
            kind={kind}
            value={value}
            watchedList={watchedList}
            onChange={onChange}
            control={control}
          />
        )}
      </Field>
    );
  }
  return (
    <Field label={field.key}>
      {(control) =>
        field.kind === "date_field" ? (
          <DateFieldSelect
            object={object}
            value={value}
            onChange={onChange}
            control={control}
          />
        ) : field.kind === "enum" ? (
          <Select
            {...control}
            options={(field.options ?? []).map((v) => ({ value: v, label: v }))}
            value={value}
            onChange={onChange}
          />
        ) : (
          <TextInput
            {...control}
            type={field.kind === "integer" ? "number" : "text"}
            min={field.min}
            max={field.max}
            required
            value={value}
            onChange={(event) => onChange(event.target.value)}
          />
        )
      }
    </Field>
  );
}

// A name plus every parameter the schema declares is one form submitted
// together; an edit seeds it from the instance instead of the schema defaults.
export function AutomationDialog({
  open,
  entry,
  initialName,
  initialParams,
  submitLabel,
  pending,
  refusal,
  onSubmit,
  onClose,
}: Readonly<{
  open: boolean;
  entry: CatalogEntry;
  initialName: string;
  initialParams?: Automation["params"];
  submitLabel: string;
  pending: boolean;
  /** Printed inside the dialog: it covers the row that would report it. */
  refusal?: ReactNode;
  onSubmit: (name: string, params: Record<string, unknown>) => void;
  onClose: () => void;
}>) {
  const t = useT();
  const formId = useId();
  return (
    <RecordFormDialog
      open={open}
      title={initialName}
      onClose={onClose}
      fieldCount={1 + paramFields(entry.params_schema).length}
      form={
        <AutomationForm
          formId={formId}
          entry={entry}
          initialName={initialName}
          initialParams={initialParams}
          refusal={refusal}
          onSubmit={onSubmit}
        />
      }
      actions={
        <>
          {/* Save started the write and goes busy; Cancel started nothing and
              is not offered while the write is out. */}
          <Button disabled={pending} onClick={onClose}>
            {t("deals.cancel")}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant="primary"
            pending={pending}
          >
            {submitLabel}
          </Button>
        </>
      }
    />
  );
}

export function AutomationForm({
  formId,
  entry,
  initialName,
  initialParams,
  refusal,
  onSubmit,
}: Readonly<{
  formId: string;
  entry: CatalogEntry;
  initialName: string;
  initialParams?: Automation["params"];
  refusal?: ReactNode;
  onSubmit: (name: string, params: Record<string, unknown>) => void;
}>) {
  const t = useT();
  const fields = paramFields(entry.params_schema);
  const [name, setName] = useState(initialName);
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      fields.map((field) => {
        const configured = initialParams?.[field.key];
        return [
          field.key,
          configured === undefined ? field.initial : scalarText(configured),
        ];
      }),
    ),
  );

  return (
    <form
      id={formId}
      className="form-stack"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(name.trim() || entry.name, paramsFromValues(fields, values));
      }}
    >
      {entry.description && <p>{entry.description}</p>}
      <p className="t-caption">{recipeSentence(entry, t)}</p>
      <Field label={t("auto.name")}>
        {(control) => (
          <TextInput
            {...control}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        )}
      </Field>
      {fields.map((field) => (
        <ParamFieldControl
          key={field.key}
          field={field}
          value={values[field.key] ?? field.initial}
          object={values.object ?? ""}
          watchedList={values.list_id ?? ""}
          onChange={(next) =>
            setValues((current) => ({ ...current, [field.key]: next }))
          }
        />
      ))}
      {refusal}
    </form>
  );
}
