import { useId, useState } from "react";
import type { components } from "../api/schema";
import { Button, Checkbox, TextInput } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { DateFieldSelect } from "./automations.datefield";
import {
  type ParamField,
  paramFields,
  paramsFromValues,
  scalarText,
} from "./automations.params";

type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];
type Automation = components["schemas"]["Automation"];

// The form an automation is named and parameterised through, and the one
// control that draws a single declared parameter. Both dialogs on this surface
// submit it — the create dialog in the admin panel and the edit dialog on a
// row — so it lives beside them rather than inside either.

function ParamFieldControl({
  field,
  formId,
  value,
  object,
  onChange,
}: Readonly<{
  field: ParamField;
  formId: string;
  value: string;
  object: string;
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
  return (
    <div className="field">
      <span className="t-label" id={`${formId}-${field.key}`}>
        {field.key}
      </span>
      {field.kind === "date_field" ? (
        <DateFieldSelect
          object={object}
          value={value}
          onChange={onChange}
          labelId={`${formId}-${field.key}`}
        />
      ) : field.kind === "enum" ? (
        <Select
          aria-labelledby={`${formId}-${field.key}`}
          options={(field.options ?? []).map((v) => ({ value: v, label: v }))}
          value={value}
          onChange={onChange}
        />
      ) : (
        <TextInput
          type={field.kind === "integer" ? "number" : "text"}
          aria-labelledby={`${formId}-${field.key}`}
          min={field.min}
          max={field.max}
          required
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </div>
  );
}

// Pick-a-template + fill-parameters (B-E15.7b1). Also serves the edit flow:
// initial values arrive from the instance instead of the schema defaults.
//
// It is the BODY of a dialog in both cases, never a panel that unfolds under a
// row: a name plus every parameter the schema declares is a form submitted
// together, and the settings page keeps a row an ANSWER by putting the form
// behind the verb. So it draws the dialog's own heading — the caller owns the
// id, since `Modal` needs it before this renders.
export function AutomationForm({
  entry,
  titleId,
  initialName,
  initialParams,
  submitLabel,
  pending,
  onSubmit,
  onCancel,
}: Readonly<{
  entry: CatalogEntry;
  /** The id `Modal`'s `labelledBy` points at; this form's heading carries it. */
  titleId: string;
  initialName: string;
  initialParams?: Automation["params"];
  submitLabel: string;
  pending: boolean;
  onSubmit: (name: string, params: Record<string, unknown>) => void;
  onCancel: () => void;
}>) {
  const t = useT();
  const formId = useId();
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
      className="form-stack"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(name.trim() || entry.name, paramsFromValues(fields, values));
      }}
    >
      {/* The dialog covers the row that would otherwise have said which
          automation is open, so the heading says it instead. */}
      <h2 className="t-h3 modal-title" id={titleId}>
        {initialName}
      </h2>
      <p className="t-caption">
        {entry.trigger} {"->"} {entry.action}
      </p>
      <div className="field">
        <span className="t-label" id={`${formId}-name`}>
          {t("auto.name")}
        </span>
        <TextInput
          aria-labelledby={`${formId}-name`}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </div>
      {fields.map((field) => (
        <ParamFieldControl
          key={field.key}
          field={field}
          formId={formId}
          value={values[field.key] ?? field.initial}
          object={values.object ?? ""}
          onChange={(next) =>
            setValues((current) => ({ ...current, [field.key]: next }))
          }
        />
      ))}
      <div className="form-actions">
        {/* Cancel first, submit last: the house submit row reads left to right
            towards the primary action. Save STARTED the write, so it goes busy
            and keeps the focus the reader is standing on; Cancel started
            nothing and is simply not available while the write is out, since
            backing out of something already on its way to the server would say
            it was stopped. */}
        <Button small disabled={pending} onClick={onCancel}>
          {t("deals.cancel")}
        </Button>
        <Button type="submit" variant="primary" small pending={pending}>
          {submitLabel}
        </Button>
      </div>
    </form>
  );
}
