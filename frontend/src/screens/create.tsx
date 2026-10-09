import {
  joinMultiselectValue,
  splitMultiselectValue,
} from "./create.multiselect";

export {
  joinMultiselectValue,
  splitMultiselectValue,
} from "./create.multiselect";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import {
  type FormEvent,
  type ReactNode,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { useIdempotencyKey } from "../api/idempotencykey";
import { navigate, type Route, type Screen } from "../app/router";
import {
  Button,
  Field,
  type FieldControl,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useSinglePress } from "../design-system/presslatch";
import { RecordFormDialog } from "../design-system/recordformdialog";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import {
  MultiSelect,
  Select,
  type SelectOption,
} from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { ProblemError, problemExistingId, problemMessageOf } from "./common";
import { catalogOfScreen, useSettledOpen } from "./create.dialog";
import {
  type NameOffers,
  OfferedNameControl,
  offeredHint,
} from "./create.offered";
import { RepeatableRowsField } from "./repeatablerowsfield";
import "./create.css";
import "./common.css";

// The shared create-record form: each screen declares its fields and keeps its
// transport. The server validates: a 422's detail renders verbatim, unreworded.

export type CreateFieldOption = { value: string; label: string };

// One subfield of a repeatable row (an emails row's `email`, `email_type`):
// a CreateField's controls, minus nesting.
export type SubField = {
  key: string;
  label: MessageKey;
  type?: "text" | "email" | "number" | "date" | "datetime-local" | "select";
  required?: boolean;
  options?: CreateFieldOption[];
  multiselectEncoding?: "json";
  placeholder?: string;
  maxLength?: number;
  // Granularity for a number input. Omitted means the browser's default of 1,
  // which rejects any fractional entry.
  step?: string;
};

export type CreateField = {
  searchTargets?: (q: string) => Promise<RecordPickerCandidate[]>;
  // A text field that offers existing records by name. See NameOffers.
  offers?: NameOffers;
  key: string;
  // Static fields carry an i18n `label` key; dynamic fields (custom fields,
  // whose labels are workspace data, not translated) carry a literal
  // `labelText` instead. Exactly one is set; the render prefers labelText.
  label?: MessageKey;
  labelText?: string;
  type?:
    | "text"
    | "email"
    | "number"
    | "date"
    | "datetime-local"
    | "select"
    | "multiselect"
    | "repeatable"
    | "textarea";
  required?: boolean;
  options?: CreateFieldOption[];
  multiselectEncoding?: "json";
  placeholder?: string;
  maxLength?: number;
  // Already translated guidance beneath the control.
  hint?: string;
  // A validation refusal blocks Save and is announced through Field.error.
  // The server remains authoritative for uniqueness and cross-record rules.
  validate?: (value: string) => string | undefined;
  // repeatable-only: each row's subfields, the add label, the primary flag's
  // key and (if set) the kind's key, which scopes primaryKey to one kind.
  rowFields?: SubField[];
  addLabel?: MessageKey;
  primaryKey?: string;
  typeKey?: string;
  // typeKey's value when unanswered — must match the request mapper's own
  // fallback, or an unset row groups differently here than once submitted.
  typeDefault?: string;
  // A heading that holds no value, setting custom fields apart from core ones.
  divider?: boolean;
  // Maps the record's raw value to the input string at prefill (minor units →
  // major); absent, the raw value is stringified.
  toInput?: (raw: unknown) => string;
  // See SubField.step: a money field must declare its cents.
  step?: string;
  // Hidden fields are neither required nor submitted.
  showWhen?: (values: Record<string, string>) => boolean;
  // Recompute dependent choices; clear values no longer valid after a change.
  optionsFor?: (values: Record<string, string>) => CreateFieldOption[];
};

// Publish after render so server-dependent choices also see seeded answers.
// The callback ref avoids publishing again solely because its identity changed.
export function usePublishedValues(
  values: Record<string, string>,
  publish?: (values: Record<string, string>) => void,
): void {
  const latest = useRef(publish);
  latest.current = publish;
  useEffect(() => {
    latest.current?.(values);
  }, [values]);
}

/** A field's choices right now: the dependent list when it has one. */
export function fieldOptions(
  field: CreateField,
  values: Record<string, string>,
): CreateFieldOption[] {
  return field.optionsFor ? field.optionsFor(values) : (field.options ?? []);
}

/**
 * The fields a form actually shows, given what has been filled in so far.
 *
 * One filter feeding both the render and the required check. A field hidden by
 * `showWhen` is absent from the form in every sense a user can observe.
 */
export function visibleFields(
  fields: CreateField[],
  values: Record<string, string>,
): CreateField[] {
  return fields.filter((field) => field.showWhen?.(values) ?? true);
}

// Blanks what showWhen hides, so a cleared partner's role is not sent orphaned;
// blanked, not dropped, since an omitted scalar keeps the stored value on edit.
export function submittedValues(
  fields: CreateField[],
  values: Record<string, string>,
): Record<string, string> {
  const shown = new Set(visibleFields(fields, values).map((f) => f.key));
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(values)) {
    const declared = fields.find((f) => f.key === key);
    // A dependent choice the new answers no longer offer is withdrawn too.
    const offered =
      !declared?.optionsFor ||
      value === "" ||
      fieldOptions(declared, values).some((option) => option.value === value);
    out[key] = (declared && !shown.has(key)) || !offered ? "" : value;
  }
  return out;
}

// After a save on a form kept open, a value still equal to what was SENT clears
// and one typed since stays; values, not a flag, hold however slow the save.
export function keepUnsubmitted(
  current: Record<string, string>,
  submitted: Record<string, string>,
  defaults: Record<string, string>,
): Record<string, string> {
  const next = { ...defaults };
  for (const [key, value] of Object.entries(current)) {
    if (value !== "" && value !== submitted[key]) {
      next[key] = value;
    }
  }
  return next;
}

// The label a top-level field shows: the literal labelText wins; otherwise the
// i18n key. (Subfields are always core, so they keep using t(label) directly.)
export function fieldLabel(
  field: CreateField,
  t: (key: MessageKey) => string,
): string {
  return field.labelText ?? (field.label ? t(field.label) : "");
}

// One repeatable-row field's collected rows, e.g. `{ email: "a@x", email_type:
// "work", is_primary: "true" }`.
export type FormRow = Record<string, string>;
// Repeatable-row values by field key, kept apart from the scalar `values` so a
// scalar-only screen's single-argument create callback keeps working.
export type FormRows = Record<string, FormRow[]>;

function rowsRequirementMet(field: CreateField, rows: FormRow[]): boolean {
  if (!field.required) {
    return true;
  }
  const required = field.rowFields ?? [];
  return rows.some((row) =>
    required.every(
      (sub) => !sub.required || (row[sub.key] ?? "").trim().length > 0,
    ),
  );
}

// The rail's WROTE head for a create here (agentrail-copy.ts). Only the three
// kinds a salesperson creates by hand have one; others leave the ticker silent.
const CREATE_MUTATION_HEAD: Readonly<Partial<Record<Screen, string>>> = {
  contacts: "contact-new",
  companies: "company-new",
  deals: "deal-new",
};

// The shared post-create choreography: refresh the list, close the modal,
// open the fresh record's 360. Screens supply only their transport.
export function useCreateRecord<Created extends { id: string }>({
  create,
  invalidate,
  screen,
  onDone,
  stay = false,
  aboutId,
  onCreated,
}: Readonly<{
  create: (
    values: Record<string, string>,
    rows: FormRows | undefined,
    idempotencyKey: string,
  ) => Promise<Created>;
  invalidate: string;
  screen: Screen;
  onDone: () => void;
  // The created record, for a caller that reports what landed. Runs before
  // onDone, so a caller can name the record in a toast while the form it came
  // from is still the thing on screen.
  onCreated?: (created: Created) => void;
  // For a create that is a PROPERTY of the record on screen (a tag, a list
  // membership): its id is not one `screen` can load, so the reader stays.
  stay?: boolean;
  // The record this create is ABOUT when not the one created: a new deal has no
  // id to name it by, so its company does. Absent, the ticker's plain phrase.
  aboutId?: string;
}>) {
  const queryClient = useQueryClient();
  const head = stay ? undefined : CREATE_MUTATION_HEAD[screen];
  return useMutation({
    mutationKey:
      head === undefined ? undefined : aboutId ? [head, aboutId] : [head],
    mutationFn: ({
      values,
      rows,
      idempotencyKey,
    }: {
      values: Record<string, string>;
      rows: FormRows;
      idempotencyKey: string;
    }) => create(values, rows, idempotencyKey),
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: [invalidate] });
      onCreated?.(created);
      onDone();
      if (!stay) {
        navigate({ screen, id: created.id });
      }
    },
  });
}

// The whole per-screen create affordance in one piece: the button, the modal,
// its open state, and the post-create choreography. A list screen supplies
// its label, fields, and transport — nothing else.
export function CreateAction<Created extends { id: string }>({
  label,
  fields,
  create,
  invalidate,
  screen,
  startOpen = false,
  resolveExisting,
  stay = false,
  aboutId,
  keepOpen = false,
  onCreated,
  onValuesChange,
  testId,
}: Readonly<{
  label: string;
  fields: CreateField[];
  // The form's live answers, for a screen whose field list depends on them —
  // a dependent picker whose options only the server can narrow. See
  // usePublishedValues.
  onValuesChange?: (values: Record<string, string>) => void;
  // The key is one per open dialog and per record. A repeat of the same save
  // carries it, so a double press answers the first record. A POST that takes
  // no Idempotency-Key ignores it.
  create: (
    values: Record<string, string>,
    rows: FormRows | undefined,
    idempotencyKey: string,
  ) => Promise<Created>;
  invalidate: string;
  screen: Screen;
  startOpen?: boolean;
  // `keepOpen` makes one save "saved, next" for capture in a run: the modal
  // empties, not closes. It implies `stay`: opening the record would end a run.
  keepOpen?: boolean;
  // What was created, for a caller that reports it — the toast naming each
  // saved record is the only feedback a form that never closes gives.
  onCreated?: (created: Created) => void;
  // Names this button when a screen carries two of them. See NewRecordButton.
  testId?: string;
  // See useCreateRecord: keep the reader on this record when what was created
  // belongs TO it rather than being somewhere to go.
  stay?: boolean;
  // See useCreateRecord: the record this create is about, when it is not the
  // record being created.
  aboutId?: string;
  // Duplicate (409) dedupe: given the problem's code + collided record id,
  // builds the route to that record. Absent screens simply never show the
  // "view existing" link.
  resolveExisting?: (code: string, id: string) => Route;
}>) {
  const t = useT();
  const [creating, setCreating] = useState(startOpen);
  const shown = useSettledOpen(creating, catalogOfScreen(screen));
  // Counts this session's saves, which empties the form between them. Not a
  // boolean: two saves in a row must read as two distinct clears.
  const [saved, setSaved] = useState(0);
  const { key: idempotencyKey, renew } = useIdempotencyKey();
  const mutation = useCreateRecord({
    create,
    invalidate,
    screen,
    stay: stay || keepOpen,
    aboutId,
    onDone: () => {
      renew();
      if (keepOpen) {
        setSaved((n) => n + 1);
        return;
      }
      setCreating(false);
    },
    onCreated,
  });
  const existing =
    mutation.error instanceof ProblemError
      ? problemExistingId(mutation.error.problem)
      : null;
  return (
    <>
      <NewRecordButton
        label={label}
        onClick={() => {
          renew();
          setCreating(true);
        }}
        testId={testId}
        pending={creating && !shown}
      />
      <CreateRecordModal
        open={shown}
        onClose={() => setCreating(false)}
        title={label}
        fields={fields}
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
        existing={existing}
        resolveExisting={resolveExisting}
        resetToken={saved}
        onValuesChange={onValuesChange}
        onSubmit={(values, rows) =>
          mutation.mutate({ values, rows: rows ?? {}, idempotencyKey })
        }
      />
    </>
  );
}

export function NewRecordButton({
  label,
  onClick,
  testId = "new-record",
  pending,
}: Readonly<{
  label: string;
  onClick: () => void;
  // Two create buttons can share a list header (full form, quick capture); a
  // shared id would make both unaddressable.
  testId?: string;
  pending?: boolean;
}>) {
  return (
    <Button onClick={onClick} data-testid={testId} pending={pending}>
      <Plus aria-hidden /> {label}
    </Button>
  );
}

// The control half of a field row; the wrapping `Field` owns the label, id and
// described-by seam, so the wiring arrives whole as `control`.
function referenceControl(
  field: CreateField,
  searchTargets: NonNullable<CreateField["searchTargets"]>,
  control: FieldControl,
  value: string,
  setValue: (next: string) => void,
  t: (key: MessageKey) => string,
): ReactNode {
  return (
    <>
      <RecordPicker
        id={control.id}
        aria-describedby={control["aria-describedby"]}
        aria-invalid={control["aria-invalid"]}
        searchTargets={searchTargets}
        selected={
          value
            ? {
                id: value,
                name:
                  field.options?.find((option) => option.value === value)
                    ?.label ?? value,
              }
            : null
        }
        onPick={(candidate) => setValue(candidate.id)}
      />
      {value && !field.required && (
        <Button variant="link" onClick={() => setValue("")}>
          {t("field.unset")}
        </Button>
      )}
    </>
  );
}

export function fieldControl(
  field: CreateField | SubField,
  control: FieldControl,
  value: string,
  setValue: (next: string) => void,
  t: (key: MessageKey) => string,
  // The form's current values, for a field whose choices depend on them.
  values: Record<string, string> = {},
): ReactNode {
  if ("searchTargets" in field && field.searchTargets)
    return referenceControl(
      field,
      field.searchTargets,
      control,
      value,
      setValue,
      t,
    );
  if (field.type === "select") {
    // Optional fields can be cleared; preserve a caller's existing empty choice.
    const options =
      "optionsFor" in field
        ? fieldOptions(field, values)
        : (field.options ?? []);
    const clearable = options.some((option) => option.value === "");
    const blank: SelectOption[] =
      field.required || clearable
        ? []
        : [{ value: "", label: t("field.unset") }];
    return (
      <Select
        {...control}
        value={value}
        onChange={setValue}
        options={[...blank, ...options]}
        // Nothing to choose from yet: the answer this list depends on has
        // not been given.
        disabled={
          "optionsFor" in field &&
          Boolean(field.optionsFor) &&
          options.length === 0
        }
      />
    );
  }
  if (field.type === "textarea") {
    return (
      <Textarea
        {...control}
        value={value}
        placeholder={field.placeholder}
        rows={3}
        maxLength={field.maxLength}
        onChange={(event) => setValue(event.target.value)}
      />
    );
  }
  return (
    <TextInput
      {...control}
      type={field.type ?? "text"}
      // A bare number input steps by 1, so the browser refuses 14.60 before
      // any handler sees it. A money field has to say it takes cents.
      step={field.step}
      maxLength={field.maxLength}
      value={value}
      placeholder={field.placeholder}
      onChange={(event) => setValue(event.target.value)}
    />
  );
}

// A multiselect field writes its set back through the scalar string channel
// (`joinMultiselectValue`).
function MultiselectField({
  field,
  value,
  setValue,
}: Readonly<{
  field: CreateField;
  value: string;
  setValue: (next: string) => void;
}>) {
  const t = useT();
  return (
    <Field
      label={fieldLabel(field, t)}
      required={field.required}
      hint={field.hint}
    >
      {(control) => (
        <MultiSelect
          {...control}
          options={field.options ?? []}
          values={splitMultiselectValue(value, field.multiselectEncoding)}
          onChange={(next) =>
            setValue(joinMultiselectValue(next, field.multiselectEncoding))
          }
          placeholder={t("field.unset")}
        />
      )}
    </Field>
  );
}

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
  existing,
  resolveExisting,
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
  // A duplicate (409)'s collided record and the screen's route to it: both
  // present, the "view existing" link renders under the error.
  existing?: { id: string; code: string } | null;
  resolveExisting?: (code: string, id: string) => Route;
  onSubmit: (values: Record<string, string>, rows?: FormRows) => void;
  onClose: () => void;
  intent: SubmitIntent;
  dialog?: { open: boolean; title: string };
}>) {
  const t = useT();
  const formId = useId();

  // A field its showWhen hides neither renders nor holds Save hostage.
  const shown = visibleFields(fields, values);
  // A write must not resurrect an answer to a question withdrawn in between:
  // the first partner's claim would carry onto a second one named after it.
  const setVisibleValues = (next: Record<string, string>) =>
    setValues(submittedValues(fields, next));
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
  const submit = singlePress((event: FormEvent) => {
    event.preventDefault();
    if (!pending && !requiredMissing && refusals.size === 0)
      onSubmit(submittedValues(fields, values), rows);
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
              setRows={(next) => setRows({ ...rows, [field.key]: next })}
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
                ? offeredHint(field.offers, field.key, values, t)
                : field.hint
            }
            error={refusals.get(field.key)}
          >
            {(control) =>
              field.offers ? (
                <OfferedNameControl
                  fieldKey={field.key}
                  type={field.type === "email" ? "email" : undefined}
                  offers={field.offers}
                  control={control}
                  values={values}
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
      {error && <ErrorLine>{error}</ErrorLine>}
      {existing && resolveExisting && (
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
      form={
        <form id={formId} className="form-stack" onSubmit={submit}>
          {stack}
        </form>
      }
      actions={actions}
    />
  );
}

export function CreateRecordModal({
  open,
  onClose,
  title,
  fields,
  pending,
  error,
  existing,
  resolveExisting,
  onSubmit,
  onValuesChange,
  resetToken = 0,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  title: string;
  fields: CreateField[];
  pending: boolean;
  error: string | null;
  existing?: { id: string; code: string } | null;
  resolveExisting?: (code: string, id: string) => Route;
  onSubmit: (values: Record<string, string>, rows?: FormRows) => void;
  // The form's live answers, for a caller driving a server read from them.
  onValuesChange?: (values: Record<string, string>) => void;
  // Bumped after a save that keeps the form open; the seeding below treats it
  // as a fresh open. A number, not a callback: the reset happens during render.
  resetToken?: number;
}>) {
  const [values, setValues] = useState<Record<string, string>>({});
  const [rows, setRows] = useState<FormRows>({});
  // What the last submit carried, so a reset can tell the saved record's words
  // apart from words typed after it while the save was still in flight.
  const [submitted, setSubmitted] = useState<Record<string, string>>({});
  // Seeded DURING RENDER on the closed→open transition, not in an effect (see
  // EditRecordModal), nor on `fields`, which a refetch renews, wiping input.
  // Starts false, not `open`: a modal mounted already open still has to seed.
  const [seededOpen, setSeededOpen] = useState(false);
  const [seededReset, setSeededReset] = useState(resetToken);
  usePublishedValues(values, onValuesChange);
  const reopened = open !== seededOpen;
  const cleared = open && resetToken !== seededReset;
  if (reopened || cleared) {
    setSeededOpen(open);
    setSeededReset(resetToken);
    if (open) {
      // A fresh open starts from the fields' defaults (first select option
      // for required selects), never from a previous attempt's leftovers.
      const defaults: Record<string, string> = {};
      for (const field of fields) {
        if (field.type === "select" && field.required) {
          defaults[field.key] = field.options?.[0]?.value ?? "";
        }
      }
      // A form kept open clears only what the save carried: words typed while
      // it was in flight belong to the next record.
      setValues((current) =>
        reopened ? defaults : keepUnsubmitted(current, submitted, defaults),
      );
      setRows({});
    }
  }

  return (
    <RecordFormBody
      dialog={{ open, title }}
      fields={fields}
      values={values}
      setValues={setValues}
      rows={rows}
      setRows={setRows}
      pending={pending}
      error={error}
      existing={existing}
      resolveExisting={resolveExisting}
      onSubmit={(sent, sentRows) => {
        setSubmitted(sent);
        onSubmit(sent, sentRows);
      }}
      onClose={onClose}
      intent="create"
    />
  );
}
