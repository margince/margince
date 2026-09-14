// A record turned into the form state that edits it.
//
// Their own file because edit.tsx crossed the 500-line cap and these five are
// the part of it with no React in them at all: a record in, form strings out,
// pure and testable without a modal. The modal keeps the STATE machine — when
// to seed, what the write's baseline is — and this keeps the projection.
//
// Two representations meet here and must not be confused. The record holds
// STORED values (a currency in minor units); the form holds STRINGS the
// controls can render. `toInput` is the one place that converts between them,
// which is why every path into the form goes through prefillField.

import type { CreateField, FormRow, FormRows } from "./create";

// One field's initial form string: a divider holds no value; a field with a
// `toInput` transform (e.g. currency minor→major) uses it; otherwise the raw
// record value is stringified, or blank when the record doesn't carry it.
function prefillField(
  field: CreateField,
  record: Record<string, unknown>,
): string {
  const current = record[field.key];
  if (field.toInput) {
    return field.toInput(current);
  }
  return current == null ? "" : String(current);
}

// The record's scalar field values as form strings, keyed by field — dividers
// hold no value and repeatable fields live in the separate rows channel, so
// both are skipped here.
export function prefillFromRecord(
  fields: CreateField[],
  record: Record<string, unknown>,
): Record<string, string> {
  const prefilled: Record<string, string> = {};
  for (const field of fields) {
    if (field.divider || field.type === "repeatable") {
      continue;
    }
    prefilled[field.key] = prefillField(field, record);
  }
  return prefilled;
}

// One repeatable field's row value coerced to the form's string-keyed rows: an
// array of row objects seeds those rows (each subfield stringified — the form
// controls only ever read/write strings); anything else starts with no rows.
function prefillRows(value: unknown): FormRow[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.map((entry) => {
    const row: FormRow = {};
    if (entry && typeof entry === "object") {
      for (const [key, cell] of Object.entries(entry)) {
        row[key] = cell == null ? "" : String(cell);
      }
    }
    return row;
  });
}

// The record's repeatable fields as prefilled rows, keyed by field — the rows
// channel's counterpart to prefillFromRecord (a field the record doesn't carry
// starts empty rather than throwing).
export function prefillRowsFromRecord(
  fields: CreateField[],
  record: Record<string, unknown>,
): FormRows {
  const rows: FormRows = {};
  for (const field of fields) {
    if (field.type === "repeatable") {
      rows[field.key] = prefillRows(record[field.key]);
    }
  }
  return rows;
}

// The fields a form has no answer for yet, seeded from the record.
//
// A screen's field list can GROW while the dialog is open: the custom-field
// catalog is a second request, so a record page that opens Edit before it
// lands renders the core fields and then gains the workspace's own. The seed
// above cannot cover them — it fires on the open transition and on a change of
// record, and a late catalog is neither — so without this the new control
// shows blank over a stored value, and the diff that follows reads that blank
// as "unchanged" and writes nothing. The reader sees an empty field and has no
// way to tell that from one nobody filled in.
//
// Only keys the values do not already carry are filled, which is what makes
// this safe to run on every render: an answer the reader typed is an entry, so
// it is never overwritten, and neither is a prefilled one. That is the same
// guarantee the transition seed protects by refusing to key off `fields`.
//
// Read from the LIVE record rather than the reading the form opened on, and
// the difference is the whole fix. Every screen builds the modal's record
// through `cf.recordSlice`, which projects the catalog's OWN column list off
// the row — so before the catalog answers that projection is empty, and the
// opening reading carries no `cf_` key at all. Seeding a late field from it
// puts "" over a stored value and changes nothing but the reader's belief.
//
// Which is safe here for the same reason the `hasOwn` check is: a key with no
// entry has nothing on screen to overwrite.
//
// TWO maps come back, and the second is not optional bookkeeping. The write
// diffs the form against `opened`, and a baseline MISSING the key is not the
// same as one holding no value — except that customFieldsToPatch normalises
// both to "" (customFieldFormValue maps null and undefined alike). So a reader
// who clears a late-seeded field submits "", the absent baseline normalises to
// "", the two compare equal, and the clear is dropped from the body while the
// save reports success. Extending the baseline as the form is seeded keeps the
// two describing one reading.
//
// `raw` holds the RECORD's own value, never the form string: toPatch converts
// the baseline itself, so handing it a converted currency converts twice — a
// stored 10000 reads as "100", re-converts to "1", and a genuine edit to 1
// compares equal and disappears (contractform.tsx learned that the hard way).
export function seedMissingFields(
  fields: CreateField[],
  record: Record<string, unknown>,
  values: Record<string, string>,
): { form: Record<string, string>; raw: Record<string, unknown> } | null {
  let seeded: {
    form: Record<string, string>;
    raw: Record<string, unknown>;
  } | null = null;
  for (const field of fields) {
    if (field.divider || field.type === "repeatable") {
      continue;
    }
    if (Object.hasOwn(values, field.key)) {
      continue;
    }
    seeded ??= { form: {}, raw: {} };
    seeded.form[field.key] = prefillField(field, record);
    seeded.raw[field.key] = record[field.key];
  }
  return seeded;
}
