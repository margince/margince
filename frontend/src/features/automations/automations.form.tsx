import { useId, useState } from "react";
import type { components } from "../../api/schema";
import { useUnsavedGuard } from "../../components/unsaved";
import { Button, Checkbox, Field, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { Select } from "../../design-system/select";
import { TimezoneSelect } from "../../design-system/timezoneselect";
import { useT } from "../../i18n";
import { type FieldLabels, refusalSentences } from "../../lib/problem";
import { templateDescription } from "./automations.catalogwords";
import { DateFieldSelect } from "./automations.datefield";
import { ListParamSelect } from "./automations.lists";
import {
  type ParamField,
  paramFields,
  paramsFromValues,
  scalarText,
} from "./automations.params";
import {
  isRequired,
  missingParam,
  optionLabel,
  paramLabel,
} from "./automations.paramwords";
import { recipeSentence } from "./automations.recipe";
import { StageParamSelect } from "./automations.stageparams";

type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];
type Automation = components["schemas"]["Automation"];

// The longest name the server stores.
const NAME_LIMIT = 120;
const NAME = "name";

// The dialog an automation is named and parameterised through, and the
// control that draws one declared parameter. Create in the admin panel and
// edit on a row both open it.

function ParamFieldControl({
  field,
  value,
  object,
  watchedList,
  pipelineId,
  error,
  onChange,
}: Readonly<{
  field: ParamField;
  value: string;
  object: string;
  /** The Live List a list rule watches, which narrows its Shortlist picker. */
  watchedList: string;
  pipelineId: string;
  error?: string;
  onChange: (value: string) => void;
}>) {
  const t = useT();
  const label = paramLabel(field, t);
  if (field.kind === "boolean") {
    return (
      <div className="field">
        <Checkbox
          label={label}
          checked={value === "true"}
          onChange={(event) =>
            onChange(event.target.checked ? "true" : "false")
          }
        />
      </div>
    );
  }
  return (
    <Field label={label} required={isRequired(field)} error={error}>
      {(control) => {
        switch (field.kind) {
          case "pipeline":
          case "stages":
            return (
              <StageParamSelect
                kind={field.kind}
                pipelineId={pipelineId}
                value={value}
                onChange={onChange}
                control={control}
              />
            );
          case "timezone":
            return (
              <TimezoneSelect {...control} value={value} onChange={onChange} />
            );
          case "live_list":
          case "shortlist":
          case "record_list":
            return (
              <ListParamSelect
                kind={field.kind}
                value={value}
                watchedList={watchedList}
                onChange={onChange}
                control={control}
                allowedTypes={field.entityTypes}
              />
            );
          case "date_field":
            return (
              <DateFieldSelect
                object={object}
                value={value}
                onChange={onChange}
                control={control}
              />
            );
          case "enum":
            return (
              <Select
                {...control}
                options={(field.options ?? []).map((option) => ({
                  value: option,
                  label: optionLabel(field, option, t),
                }))}
                value={value}
                onChange={onChange}
              />
            );
          default:
            return (
              <TextInput
                {...control}
                type={field.kind === "integer" ? "number" : "text"}
                min={field.min}
                max={field.max}
                value={value}
                onChange={(event) => onChange(event.target.value)}
              />
            );
        }
      }}
    </Field>
  );
}

// Inside the dialog, so the claim reaches the dialog's own leave question.
function DraftGuard({ dirty }: Readonly<{ dirty: boolean }>) {
  useUnsavedGuard(dirty);
  return null;
}

function seedValues(
  fields: readonly ParamField[],
  initialParams: Automation["params"] | undefined,
): Record<string, string> {
  return Object.fromEntries(
    fields.map((field) => {
      const configured = initialParams?.[field.key];
      return [
        field.key,
        configured === undefined ? field.initial : scalarText(configured),
      ];
    }),
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
  error,
  onSubmit,
  onClose,
}: Readonly<{
  open: boolean;
  entry: CatalogEntry;
  initialName: string;
  initialParams?: Automation["params"];
  submitLabel: string;
  pending: boolean;
  /** The server's refusal of the last submit, drawn inside the dialog. */
  error?: unknown;
  onSubmit: (name: string, params: Record<string, unknown>) => void;
  onClose: () => void;
}>) {
  const t = useT();
  const formId = useId();
  const fields = paramFields(entry.params_schema);
  const [name, setName] = useState(initialName);
  const [nameBlank, setNameBlank] = useState(false);
  const [seeded] = useState(() => seedValues(fields, initialParams));
  const [values, setValues] = useState(seeded);
  const dirty =
    name !== initialName ||
    Object.entries(values).some(([key, value]) => value !== seeded[key]);
  const labels: FieldLabels = {
    [NAME]: t("auto.name"),
    ...Object.fromEntries(
      fields.map((field) => [`params.${field.key}`, paramLabel(field, t)]),
    ),
  };
  const refused = refusalSentences(error, t, labels);
  const missing = missingParam(fields, values, t);

  return (
    <RecordFormDialog
      open={open}
      title={initialName}
      onClose={onClose}
      fieldCount={1 + fields.length}
      form={
        <form
          id={formId}
          className="form-stack"
          // The form words its own refusals at the field; a browser bubble
          // would say something else, once, and nowhere a reader can return to.
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            const trimmed = name.trim();
            if (trimmed === "") {
              setNameBlank(true);
              return;
            }
            onSubmit(trimmed, paramsFromValues(fields, values));
          }}
        >
          {templateDescription(entry, t) && (
            <p>{templateDescription(entry, t)}</p>
          )}
          <p className="t-caption">{recipeSentence(entry, t)}</p>
          <Field
            label={t("auto.name")}
            required
            error={
              nameBlank ? t("auto.nameRequired") : refused.byField.get(NAME)
            }
          >
            {(control) => (
              <TextInput
                {...control}
                maxLength={NAME_LIMIT}
                value={name}
                onChange={(event) => {
                  setNameBlank(false);
                  setName(event.target.value);
                }}
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
              pipelineId={values.pipeline_id ?? ""}
              error={refused.byField.get(`params.${field.key}`)}
              onChange={(next) =>
                setValues((current) => ({
                  ...current,
                  ...(field.kind === "pipeline" && next !== current[field.key]
                    ? { stage_ids: "[]" }
                    : {}),
                  [field.key]: next,
                }))
              }
            />
          ))}
          <ErrorLine>{refused.rest}</ErrorLine>
          <DraftGuard dirty={dirty} />
        </form>
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
            reason={missing}
          >
            {submitLabel}
          </Button>
        </>
      }
    />
  );
}
