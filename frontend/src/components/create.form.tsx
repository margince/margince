import {
  type FormEvent,
  type ReactNode,
  useEffect,
  useId,
  useState,
} from "react";
import { Button, Field } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { refuseUnreadableField } from "../design-system/formvalidity";
import { useSinglePress } from "../design-system/presslatch";
import { RecordFormDialog } from "../design-system/recordformdialog";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { navigate, type Route } from "../lib/router";
import { fieldControl, MultiselectField } from "./create.controls";
import {
  type CreateField,
  type FormRows,
  fieldLabel,
  rowsRequirementMet,
  submittedValues,
  visibleFields,
} from "./create.fields";
import {
  OfferedNameControl,
  offeredHint,
  usePrefilledPick,
} from "./create.offered";
import { RepeatableRowsField } from "./repeatablerowsfield";

// A submit button's resting and busy labels travel as one pair, so an edit
// can never announce "Creating…" while it saves.
export const SUBMIT_COPY = {
  create: { label: "create.save", busy: "create.saving" },
  save: { label: "record.save", busy: "common.saving" },
} as const satisfies Record<string, { label: MessageKey; busy: MessageKey }>;

export type SubmitIntent = keyof typeof SUBMIT_COPY;

// Fields, error and Cancel/Save, shared by create and edit: only the values'
// origin and the submit intent differ, and those stay with each modal's owner.
export function RecordFormBody({
  fields,
  values,
  setValues,
  rows,
  setRows,
  pending,
  error,
  fieldRefusals,
  existing,
  resolveExisting,
  notice,
  onSubmit,
  onClose,
  intent,
  dialog,
}: Readonly<{
  fields: CreateField[];
  values: Record<string, string>;
  setValues: (next: Record<string, string>) => void;
  rows: FormRows;
  setRows: (next: FormRows) => void;
  pending: boolean;
  error: string | null;
  // What the server refused per field, shown beside the field it names.
  fieldRefusals?: ReadonlyMap<string, string>;
  // A duplicate (409)'s collided record and the screen's route to it: both
  // present, the "view existing" link renders under the error.
  existing?: { id: string; code: string } | null;
  resolveExisting?: (code: string, id: string) => Route;
  // A refusal the screen answers with its own verbs, drawn in place of the
  // error line and the "view existing" link.
  notice?: ReactNode;
  onSubmit: (values: Record<string, string>, rows?: FormRows) => void;
  onClose: () => void;
  intent: SubmitIntent;
  dialog?: { open: boolean; title: string };
}>) {
  const t = useT();
  const formId = useId();

  // Laid over the reader's values where the form shows and sends them. Other
  // fields write from the reader's own, so the box stays untouched until used.
  const prefilled = usePrefilledPick(fields, values, rows);
  const answers = { ...values, ...prefilled };
  // A field its showWhen hides neither renders nor holds Save hostage.
  const shown = visibleFields(fields, values);
  // A write must not bring back an answer to a question withdrawn in between.
  // Otherwise the first partner's claim would carry onto a second one named
  // after it.
  const [edited, setEdited] = useState(false);
  const [editedOpen, setEditedOpen] = useState(dialog?.open);
  // Each opening starts unchanged; a close that is not a save asks only after an edit.
  if (dialog && dialog.open !== editedOpen) {
    setEditedOpen(dialog.open);
    setEdited(false);
  }
  const setVisibleValues = (next: Record<string, string>) => {
    setEdited(true);
    setValues(submittedValues(fields, next));
  };
  const requiredMissing = shown.some((field) => {
    if (field.type === "repeatable") {
      return !rowsRequirementMet(field, rows[field.key] ?? []);
    }
    return field.required && !(values[field.key] ?? "").trim();
  });
  const refusals = new Map(
    shown.flatMap((field) => {
      const refusal = field.validate?.(values[field.key] ?? "");
      return refusal ? [[field.key, refusal] as const] : [];
    }),
  );
  // A refusal lands at the foot of a long stack, out of sight of a pinned Save.
  useEffect(() => {
    if (error) {
      const last = document.getElementById(formId)?.lastElementChild;
      last?.scrollIntoView?.({ block: "nearest" });
    }
  }, [error, formId]);

  // Enter in a field submits without touching Save, so the form holds the
  // same latch the button does.
  const singlePress = useSinglePress(pending);
  const submit = singlePress((event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (
      !pending &&
      !requiredMissing &&
      refusals.size === 0 &&
      !refuseUnreadableField(event.currentTarget)
    )
      onSubmit(submittedValues(fields, answers), rows);
  });
  const stack = (
    <>
      {shown.map((field) => {
        if (field.divider) {
          return (
            <p className="form-divider t-label" key={field.key}>
              {fieldLabel(field, t)}
            </p>
          );
        }
        if (field.type === "repeatable") {
          return (
            <RepeatableRowsField
              key={field.key}
              field={field}
              formId={formId}
              rows={rows[field.key] ?? []}
              setRows={(next) => {
                setEdited(true);
                setRows({ ...rows, [field.key]: next });
              }}
            />
          );
        }
        if (field.type === "multiselect") {
          return (
            <MultiselectField
              key={field.key}
              field={field}
              value={values[field.key] ?? ""}
              setValue={(next) =>
                setVisibleValues({ ...values, [field.key]: next })
              }
            />
          );
        }
        return (
          <Field
            key={field.key}
            label={fieldLabel(field, t)}
            required={field.required}
            hint={
              field.offers
                ? offeredHint(
                    field.offers,
                    field.key,
                    answers,
                    t,
                    Object.hasOwn(prefilled, field.key),
                  )
                : field.hint
            }
            error={refusals.get(field.key) ?? fieldRefusals?.get(field.key)}
          >
            {(control) =>
              field.offers ? (
                <OfferedNameControl
                  fieldKey={field.key}
                  type={field.type === "email" ? "email" : undefined}
                  offers={field.offers}
                  control={control}
                  values={answers}
                  setValues={setVisibleValues}
                />
              ) : (
                fieldControl(
                  field,
                  control,
                  values[field.key] ?? "",
                  (next) => setVisibleValues({ ...values, [field.key]: next }),
                  t,
                  values,
                )
              )
            }
          </Field>
        );
      })}
      {/* Announced: nothing moves when a submit is refused, and the server's
          reason is the only thing saying why the form is still open. */}
      {notice}
      {!notice && error && <ErrorLine>{error}</ErrorLine>}
      {!notice && existing && resolveExisting && (
        <Button
          type="button"
          className="create-view-existing"
          onClick={() => navigate(resolveExisting(existing.code, existing.id))}
        >
          {t("dedupe.viewExisting")}
        </Button>
      )}
    </>
  );
  const actions = (
    <>
      <Button type="button" onClick={onClose}>
        {t("create.cancel")}
      </Button>
      <Button
        variant="primary"
        type="submit"
        form={formId}
        disabled={!pending && (requiredMissing || refusals.size > 0)}
        pending={pending}
        busyLabel={t(SUBMIT_COPY[intent].busy)}
      >
        {t(SUBMIT_COPY[intent].label)}
      </Button>
    </>
  );
  if (!dialog) {
    return (
      <form id={formId} onSubmit={submit}>
        <div className="form-stack">{stack}</div>
        <div className="actions">{actions}</div>
      </form>
    );
  }
  return (
    <RecordFormDialog
      open={dialog.open}
      title={dialog.title}
      onClose={onClose}
      fieldCount={fields.filter((field) => !field.divider).length}
      unsaved={edited}
      form={
        <form id={formId} className="form-stack" onSubmit={submit}>
          {stack}
        </form>
      }
      actions={actions}
    />
  );
}
