import { type ReactNode, useEffect, useRef } from "react";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import type { MessageKey } from "../i18n/en";
import type { NameOffers } from "./create.offered";

export type CreateFieldOption = { value: string; label: string };

// One subfield of a repeatable row (an emails row's `email`, `email_type`):
// a CreateField's controls, minus nesting.
export type SubField = {
  key: string;
  label: MessageKey;
  type?: "text" | "email" | "number" | "date" | "select";
  required?: boolean;
  options?: CreateFieldOption[];
  multiselectEncoding?: "json";
  placeholder?: string;
  maxLength?: number;
  // Granularity for a number input. Omitted means the browser's default of 1,
  // which rejects any fractional entry.
  step?: string;
  // See CreateField.min.
  min?: string;
};

export type CreateField = {
  searchTargets?: (q: string) => Promise<RecordPickerCandidate[]>;
  // A text field that offers existing records by name. See NameOffers.
  offers?: NameOffers;
  key: string;
  // Static fields carry an i18n `label` key; dynamic fields (custom fields,
  // whose labels are workspace data, not translated) carry a literal
  // `labelText` instead. One of the two is set; the render prefers labelText.
  label?: MessageKey;
  labelText?: string;
  type?:
    | "text"
    | "email"
    | "number"
    | "date"
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
  hint?: ReactNode;
  // A validation refusal blocks Save and is announced through Field.error.
  // The server remains authoritative for uniqueness and cross-record rules.
  validate?: (value: string) => string | undefined;
  // repeatable-only: each row's subfields, the add label, the primary flag's
  // key and (if set) the kind's key, which scopes primaryKey to one kind.
  rowFields?: SubField[];
  addLabel?: MessageKey;
  // The verb that opens the list's editor, naming the list: "Change Email"
  // reads as one address, where the editor also adds and removes them.
  editLabel?: MessageKey;
  primaryKey?: string;
  typeKey?: string;
  // typeKey's value when unanswered. It must match the request mapper's own
  // fallback, or an unset row groups differently here than once submitted.
  typeDefault?: string;
  // A heading that holds no value, setting custom fields apart from core ones.
  divider?: boolean;
  // Maps the record's raw value to the input string at prefill (minor units →
  // major); absent, the raw value is stringified.
  toInput?: (raw: unknown) => string;
  // See SubField.step: a money field must declare its cents.
  step?: string;
  // Lowest value a number input accepts. A money amount that cannot be below
  // zero declares it, so the browser refuses a minus before the server does.
  min?: string;
  // The API field a refusal names when it differs from `key`: an amount typed
  // in major units is sent as amount_minor.
  wireField?: string;
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

// Blanks what showWhen hides, so a cleared partner's role is not sent orphaned.
// It blanks rather than drops, since an omitted scalar keeps the stored value
// on edit.
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

// After a save on a form kept open, a value still equal to what was sent
// clears, and one typed since stays. Comparing values rather than a flag holds
// however slow the save is.
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

export function rowsRequirementMet(
  field: CreateField,
  rows: FormRow[],
): boolean {
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
