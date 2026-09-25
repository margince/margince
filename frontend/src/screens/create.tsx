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
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { navigate, type Route, type Screen } from "../app/router";
import {
  Button,
  Field,
  type FieldControl,
  Modal,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
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
import {
  type NameOffers,
  OfferedNameControl,
  offeredHint,
} from "./create.offered";
import { RepeatableRowsField } from "./repeatablerowsfield";
import "./create.css";
import "./common.css";

// The shared create-record form (contacts, companies, leads, deals): each
// list screen declares its fields; the transport (which endpoint, how values
// map onto the request body) stays with the screen that owns the resource.
// Server-side validation is the truth — a 422 renders its RFC 7807 detail
// verbatim under the form, never a swallowed or re-worded error.

export type CreateFieldOption = { value: string; label: string };

// One subfield within a repeatable row (e.g. an emails row's `email` and
// `email_type`) — reuses the same control types as a top-level CreateField,
// minus repeatable-ness itself (rows don't nest).
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
  // repeatable-only: the subfields each row renders, the "add row" button's
  // label, which subfield key holds the row's primary flag, and (if set)
  // which one holds its kind — scoping primaryKey to same-kind rows.
  rowFields?: SubField[];
  addLabel?: MessageKey;
  primaryKey?: string;
  typeKey?: string;
  // typeKey's value when unanswered — must match the request mapper's own
  // fallback, or an unset row groups differently here than once submitted.
  typeDefault?: string;
  // A non-input group divider (renders its labelText as a heading, holds no
  // value) — used to set custom fields apart from core fields.
  divider?: boolean;
  // Optional read transform: maps the record's raw value to the input string
  // at prefill time (e.g. currency minor units → major units). Absent means
  // the raw value is stringified as-is.
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

/**
 * What the form sends: every value except those belonging to a field its own
 * `showWhen` currently hides.
 *
 * A hidden field's value is blanked rather than carried. Answering a question
 * and then withdrawing the one it depended on must not submit the orphaned
 * answer — choosing a partner, saying what they did, then clearing the partner
 * would otherwise send an attribution with nobody to attribute it to, which
 * the server refuses. Blanked rather than dropped, because a scalar the form
 * omits entirely leaves the stored value in place on an edit, and the reader
 * asked for it to be gone.
 */
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

// What survives a save on a form that stays open: a field whose value is still
// exactly what was sent is cleared to its default, and a field the reader has
// since changed keeps what they typed.
//
// The comparison is against the SUBMITTED value rather than a timestamp or a
// dirty flag, because that is the question being asked — "is this still the
// saved record's word, or the next one's?" — and it answers correctly however
// slow the round trip was.
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
// Repeatable-row values, keyed by the field's key — the SECOND channel: it
// exists alongside `values: Record<string, string>` (never merged into it) so
// every existing scalar-only screen and its single-arg create callback keeps
// working untouched.
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

// The agent rail's WROTE head for a create landing on this screen (agentrail-
// copy.ts). Only the three record kinds a salesperson creates by hand carry
// one; every other screen this hook also serves (products, offer templates,
// pipeline stages, webhooks...) gets none, which is what leaves the ticker
// silent for those.
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
  create: (values: Record<string, string>, rows?: FormRows) => Promise<Created>;
  invalidate: string;
  screen: Screen;
  onDone: () => void;
  // The created record, for a caller that reports what landed. Runs before
  // onDone, so a caller can name the record in a toast while the form it came
  // from is still the thing on screen.
  onCreated?: (created: Created) => void;
  // `stay` keeps the reader where they are instead of opening what was just
  // created. It is for creates whose result is a PROPERTY of the record on
  // screen — a tag on this company, a list this company now belongs to —
  // rather than a record worth visiting. Without it those creates route to
  // `screen` with the new row's id, which is not an id that screen can load.
  stay?: boolean;
  // The record this create is ABOUT, when it differs from the record being
  // created: a deal opened from a company page is about that company, and
  // the deal has no id yet to name it by. Absent means the created record
  // has no name to offer either (a fresh contact/company/deal has none
  // until the server answers), so the ticker falls back to its plain phrase.
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
    }: {
      values: Record<string, string>;
      rows: FormRows;
    }) => create(values, rows),
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
  create: (values: Record<string, string>, rows?: FormRows) => Promise<Created>;
  invalidate: string;
  screen: Screen;
  startOpen?: boolean;
  // `keepOpen` turns one save into "saved, next": the modal stays open and
  // empties itself instead of closing. It is for capture done in a run —
  // somebody reading a list of profiles in another window types six contacts
  // without reopening the form six times. It implies `stay`, because opening
  // the record just created would be the opposite of staying to type the next
  // one.
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
  // Counts the saves this open session has taken, and is what empties the form
  // between them. A counter rather than a boolean: two saves in a row have to
  // read as two distinct clears, and a flag toggled back would leave the
  // second one looking like the state the first already settled.
  const [saved, setSaved] = useState(0);
  const mutation = useCreateRecord({
    create,
    invalidate,
    screen,
    stay: stay || keepOpen,
    aboutId,
    onDone: () => {
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
        onClick={() => setCreating(true)}
        testId={testId}
      />
      <CreateRecordModal
        open={creating}
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
          mutation.mutate({ values, rows: rows ?? {} })
        }
      />
    </>
  );
}

export function NewRecordButton({
  label,
  onClick,
  testId = "new-record",
}: Readonly<{
  label: string;
  onClick: () => void;
  // Two create buttons can sit in one list header — the full form and a
  // quick-capture beside it — and a shared id makes both unaddressable to a
  // test and to anything else querying by it. The default keeps every existing
  // single-button screen exactly as it was.
  testId?: string;
}>) {
  return (
    <Button onClick={onClick} data-testid={testId}>
      <Plus aria-hidden /> {label}
    </Button>
  );
}

// The control half of a field row. The label half — and with it the id, the
// required marker and the described-by seam — belongs to the `Field` that
// wraps this, which is why the wiring arrives whole as `control` rather than
// being rebuilt from a field id here.
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
        label={fieldLabel(field, t)}
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

// A multiselect field: a MultiSelect dropdown whose toggled set re-joins back
// into `values` via `setValue` — the same single-string channel every scalar
// field writes through (see `splitMultiselectValue`/`joinMultiselectValue`
// above).
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

// The shared modal form body: fields → controls, the error paragraph, and
// the Cancel/Save row. Both create and edit render this identically — only
// the values' origin (empty defaults vs. a prefilled record) and the submit
// intent differ, and those stay with each modal's owner.
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
}: Readonly<{
  fields: CreateField[];
  values: Record<string, string>;
  setValues: (next: Record<string, string>) => void;
  rows: FormRows;
  setRows: (next: FormRows) => void;
  pending: boolean;
  error: string | null;
  // The collided record from a duplicate (409) problem, and the screen's
  // mapping from its code + id to a Route — both present renders the "view
  // existing" link right under the error message.
  existing?: { id: string; code: string } | null;
  resolveExisting?: (code: string, id: string) => Route;
  onSubmit: (values: Record<string, string>, rows?: FormRows) => void;
  onClose: () => void;
  intent: SubmitIntent;
}>) {
  const t = useT();
  const formId = useId();

  // A field hidden by its own showWhen is absent from the form in every sense:
  // it neither renders nor holds Save hostage to a value nobody was asked for.
  const shown = visibleFields(fields, values);
  // Writing a value must not resurrect an answer to a question that was
  // withdrawn in between. Naming a partner, saying what they did, clearing the
  // partner and naming a DIFFERENT one would otherwise carry the first
  // partner's claim onto the second — and "influenced" silently earns them
  // nothing where the default would have paid.
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

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (!pending && !requiredMissing && refusals.size === 0)
          onSubmit(submittedValues(fields, values), rows);
      }}
      className="form-stack"
    >
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
                ? offeredHint(field.offers, values[field.key] ?? "", values, t)
                : field.hint
            }
            error={refusals.get(field.key)}
          >
            {(control) =>
              field.offers ? (
                <OfferedNameControl
                  fieldKey={field.key}
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
      {error && (
        // Announced, so a refused submit reaches a reader whose focus never
        // left the form: nothing moves when this appears, and the server's
        // reason is the only thing that says why the dialog is still open. The
        // edit dialog renders this same body, so both carry it.
        <ErrorLine>{error}</ErrorLine>
      )}
      {existing && resolveExisting && (
        <Button
          type="button"
          className="create-view-existing"
          onClick={() => navigate(resolveExisting(existing.code, existing.id))}
        >
          {t("dedupe.viewExisting")}
        </Button>
      )}
      <div className="actions">
        <Button type="button" onClick={onClose}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          type="submit"
          disabled={!pending && (requiredMissing || refusals.size > 0)}
          pending={pending}
          busyLabel={t(SUBMIT_COPY[intent].busy)}
        >
          {t(SUBMIT_COPY[intent].label)}
        </Button>
      </div>
    </form>
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
  // The form's live answers, published so a caller can drive a SERVER read
  // from them. See the note on the shared publisher below.
  onValuesChange?: (values: Record<string, string>) => void;
  // Emptying the form WITHOUT closing it, for a modal that stays open to take
  // the next record. The caller bumps this after a save it kept open, and the
  // seeding below treats it exactly like a fresh open. A number rather than a
  // callback because the reset has to happen during render for the same reason
  // the open-transition one does — a caller reaching in to clear the values
  // would be the effect this shape exists to avoid.
  resetToken?: number;
}>) {
  const headingId = useId();
  const [values, setValues] = useState<Record<string, string>>({});
  const [rows, setRows] = useState<FormRows>({});
  // What the last submit carried, so a reset can tell the saved record's words
  // apart from words typed after it while the save was still in flight.
  const [submitted, setSubmitted] = useState<Record<string, string>>({});
  // Seeding happens DURING RENDER on the closed→open transition, not in an
  // effect — see EditRecordModal (edit.tsx) for the race an effect opens and
  // why this shape closes it. Keying off the transition (rather than `fields`,
  // a non-primitive prop a background refetch or locale change can re-identify
  // at any moment) is what keeps a re-render from wiping live input.
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
      // A form that stays open to take the next record clears only what the
      // save it just made carried. Nothing disables the fields during the
      // round trip, so a reader who kept typing while it was in flight has
      // words on screen that belong to the NEXT contact — and blanking the
      // whole form would take them with it. `submitted` is what went; anything
      // typed after it stays exactly where the reader put it.
      setValues((current) =>
        reopened ? defaults : keepUnsubmitted(current, submitted, defaults),
      );
      setRows({});
    }
  }

  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId}>
      <Heading size="large" id={headingId} className="t-h2 dialog-heading">
        {title}
      </Heading>
      <RecordFormBody
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
    </Modal>
  );
}
