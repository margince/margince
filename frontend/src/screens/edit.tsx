import { useMutation, useQueryClient } from "@tanstack/react-query";
import { PenLine } from "lucide-react";
import { useId, useState } from "react";
import type { Route } from "../app/router";
import { Button, Modal } from "../design-system/atoms";
import { IconAction } from "../design-system/iconaction";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { derivedRecordKeys } from "./activitykeys";
import {
  isVersionSkew,
  ProblemError,
  problemExistingId,
  problemMessageOf,
} from "./common";
import {
  type CreateField,
  type FormRow,
  type FormRows,
  RecordFormBody,
  usePublishedValues,
} from "./create";

// The agent rail's WROTE head for an edit, keyed by `recordKey` (agentrail-
// copy.ts). Only the four record kinds a salesperson edits by hand carry
// one; every other screen this hook also serves (products, offer templates,
// quotas, relationships, webhooks, pipeline stages...) gets none.
const EDIT_MUTATION_HEAD: Readonly<Partial<Record<string, string>>> = {
  organization: "company-edit",
  person: "contact-edit",
  deal: "deal-edit",
  lead: "lead-edit",
};

// The shared post-update choreography: run the screen-supplied PATCH, then
// refresh both the list and the specific record so the 360 reflects the new
// version. A 409 version_skew surfaces as mutation.error (rendered by the form),
// never a silent overwrite.
export function useUpdateRecord<Updated extends { id: string }>({
  update,
  invalidate,
  recordKey,
  recordId,
  savedMessage,
  onDone,
}: Readonly<{
  update: (
    values: Record<string, unknown>,
    rows?: FormRows,
    opened?: Record<string, unknown> & { id: string; version?: number },
  ) => Promise<Updated>;
  invalidate: string;
  recordKey: string;
  // The id of the record being edited, known up front unlike a create: used
  // only to name the agent rail's line, never the transport.
  recordId?: string;
  // What the reader is told once it has landed, already translated. REQUIRED:
  // the dialog closing is the caller dismissing its own form, not the server
  // agreeing to anything, and an edit that changes a field the reader cannot
  // see behind the dialog left them with no evidence either way.
  //
  // A FUNCTION wherever the sentence names the record, because the edit may be
  // what changed that name: built from the row the form opened on, renaming
  // "Discovery" to "Qualification" announced "Discovery saved". It is handed
  // what the server returned, which is the only version of the record that
  // reflects the write being confirmed. A plain string stays right for a
  // sentence that names a KIND rather than an instance.
  //
  // `NoInfer` so this parameter does not decide what `Updated` is. Inference
  // reads every position at once, and a callback here dragged the type down to
  // the constraint — leaving every caller with an `{ id: string }` that has no
  // name to read. What the record is comes from `update`, which is what
  // actually returns it.
  savedMessage: string | ((updated: NoInfer<Updated>) => string);
  onDone: () => void;
}>) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const head = EDIT_MUTATION_HEAD[recordKey];
  return useMutation({
    mutationKey:
      head === undefined ? undefined : recordId ? [head, recordId] : [head],
    mutationFn: ({
      values,
      rows,
      opened,
    }: {
      values: Record<string, unknown>;
      rows: FormRows;
      // The reading the form prefilled from, carried through the mutation so
      // the write's baseline and version are the ones the person saw.
      opened?: Record<string, unknown> & { id: string; version?: number };
    }) => update(values, rows, opened),
    onSuccess: (updated) => {
      queryClient.invalidateQueries({ queryKey: [invalidate] });
      queryClient.invalidateQueries({ queryKey: [recordKey, updated.id] });
      // A read WRITTEN FROM this record rather than showing it goes stale on
      // the same write, and says something confident while it does. Derived
      // rather than listed here, so a new one is picked up by every edit form
      // at once.
      for (const queryKey of derivedRecordKeys(recordKey, updated.id)) {
        queryClient.invalidateQueries({ queryKey });
      }
      onDone();
      toast.show(
        typeof savedMessage === "function"
          ? savedMessage(updated)
          : savedMessage,
      );
    },
  });
}

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
function prefillFromRecord(
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
function prefillRowsFromRecord(
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

// The edit modal: prefilled from the record's current field values (each
// field's key projected off the record, coerced to a string; a field the
// record doesn't carry starts blank rather than throwing). The screen's
// `update` callback — not this form — builds the PATCH body and attaches
// `ifMatch(record.version)`, so this stays resource-agnostic.
export function EditRecordModal({
  open,
  onClose,
  title,
  notice,
  fields,
  record,
  pending,
  error,
  existing,
  resolveExisting,
  onSubmit,
  onValuesChange,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  title: string;
  // A one-sentence advisory shown above the form fields, e.g. overlay mode's
  // partial-write-back warning. Optional so a plain edit carries no empty
  // banner.
  notice?: string;
  fields: CreateField[];
  record: Record<string, unknown> & { id: string; version?: number };
  pending: boolean;
  error: string | null;
  existing?: { id: string; code: string } | null;
  resolveExisting?: (code: string, id: string) => Route;
  // The form's answers, and the record they were PREFILLED FROM.
  //
  // Everything the write compares against has to describe ONE reading: the
  // values on screen, the baseline the diff is taken against, and the version
  // the If-Match carries. `record` is recomputed on every render, so a
  // background refetch mid-edit moves the last two while the first stays as
  // the person left it — and then the diff reports somebody else's change as
  // this person's edit, and the fresh version makes the server's concurrency
  // check pass on the write that overwrites it.
  //
  // Carried ON the submit rather than published when it is taken, so it is
  // never a side effect of rendering: a render React discards or replays must
  // not be able to hand a caller a reading the form never showed.
  onSubmit: (
    values: Record<string, string>,
    rows: FormRows | undefined,
    opened: Record<string, unknown> & { id: string; version?: number },
  ) => void;
  // The form's live answers, published so a caller can drive a SERVER read
  // from them — the narrowing optionsFor cannot do, because it is a pure
  // function of the values. See usePublishedValues (create.tsx).
  onValuesChange?: (values: Record<string, string>) => void;
}>) {
  const headingId = useId();
  const [values, setValues] = useState<Record<string, string>>({});
  usePublishedValues(values, onValuesChange);
  // Repeatable-row fields prefill from the record's current rows (e.g. a
  // company's domains) so an edit starts from the live set rather than blank.
  const [rows, setRows] = useState<FormRows>({});
  // The prefill runs DURING RENDER on the closed→open transition, never in an
  // effect. An effect runs only after the browser has painted the open modal,
  // so for that gap the inputs are on screen — focused and typeable — holding
  // blanks the record's real values have not reached yet. Whatever the user
  // types into that gap is written through empty form state and then thrown
  // away by the prefill that lands a commit later: the field snaps back to the
  // record's old value and Save carries the edit the user never made. Seeding
  // during render removes the gap entirely — the inputs' very first commit
  // already carries the record's values.
  //
  // The transition — not `record`/`fields`, non-primitive props a background
  // refetch or locale change can re-identify while the modal stays open — is
  // what this keys off, so a re-render never wipes what the user is typing.
  // Starts false, not `open`: a modal mounted already open still has to seed.
  const [seededOpen, setSeededOpen] = useState(false);
  // WHICH record the values above belong to. A screen can swap the record
  // under an open dialog without remounting it — the quota rail does — and
  // then the form is showing one record's values while the caller's write
  // addresses another. Re-seeding on identity is not the same trade as
  // re-seeding on every render: keeping what the person typed is only worth
  // anything while it is about the record they are still editing.
  const [seededFor, setSeededFor] = useState<string | null>(null);
  // The reading the values were taken from, kept for the write. Set in the
  // same transition as they are, so the three cannot describe different
  // moments of the record.
  const [opened, setOpened] = useState(record);
  if (open !== seededOpen || (open && record.id !== seededFor)) {
    setSeededOpen(open);
    setSeededFor(open ? record.id : null);
    if (open) {
      // A fresh open starts from the record's current values, never a
      // previous attempt's leftovers.
      setValues(prefillFromRecord(fields, record));
      setRows(prefillRowsFromRecord(fields, record));
      setOpened(record);
    }
  }

  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId}>
      <h2 id={headingId} className="t-h2" style={{ marginBottom: 12 }}>
        {title}
      </h2>
      {notice && (
        <p className="t-caption" style={{ marginBottom: "var(--space-3)" }}>
          {notice}
        </p>
      )}
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
        onSubmit={(submitted, submittedRows) =>
          onSubmit(submitted, submittedRows, opened)
        }
        onClose={onClose}
        submitLabelKey="record.save"
      />
    </Modal>
  );
}

// The whole per-screen edit affordance in one piece: the trigger button, the
// prefilled modal, its open state, and the If-Match update choreography
// (useUpdateRecord above). A screen supplies its label, fields, the record to
// prefill from, and its transport — nothing else.
export function EditAction<Updated extends { id: string }>({
  label,
  notice,
  fields,
  record,
  update,
  invalidate,
  recordKey,
  savedMessage,
  resolveExisting,
  disabledReasonId,
  labelled,
  onValuesChange,
}: Readonly<{
  label: string;
  /**
   * Draw the verb with its WORDS rather than as a bare pencil.
   *
   * The square below is right where the glyph sits among other glyphs and the
   * header's width is what it costs. It is wrong where the verb sits among
   * SENTENCES — a list of named actions with one unnamed box in it, and the
   * box is the only row a reader has to hover to identify. Same `label`, so
   * the two forms cannot come to say different things about the same verb.
   */
  labelled?: boolean;
  // Why this action is unavailable, when it is. STATE-4a settles the
  // absent-vs-disabled question by CAUSE: a control blocked by STATE
  // rather than permission — an archived record — stays visible and
  // disabled WITH the reason, because the reason is the information and
  // hiding the control hides a fact the reader needs.
  disabledReasonId?: string;

  // See EditRecordModal — an optional one-sentence advisory over the form.
  notice?: string;
  fields: CreateField[];
  record: Record<string, unknown> & { id: string; version?: number };
  update: (
    values: Record<string, unknown>,
    rows?: FormRows,
    // The record as it was when this edit OPENED — the same reading the form
    // prefilled from. The If-Match version and any diff baseline come from
    // here, never from the live prop: it is taken as the `opened` argument on
    // EditRecordModal's onSubmit.
    opened?: Record<string, unknown> & { id: string; version?: number },
  ) => Promise<Updated>;
  invalidate: string;
  recordKey: string;
  // What the reader is told once it has landed. See `useUpdateRecord`.
  savedMessage: string | ((updated: NoInfer<Updated>) => string);
  // Symmetric with CreateAction's dedupe link — edit rarely collides, but the
  // API stays uniform for the screens that adopt it.
  resolveExisting?: (code: string, id: string) => Route;
  // The form's live answers, for a screen whose field list depends on them —
  // a dependent picker whose options only the server can narrow. See
  // usePublishedValues (create.tsx).
  onValuesChange?: (values: Record<string, string>) => void;
}>) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const mutation = useUpdateRecord<Updated>({
    update: (values, rows, opened) => update(values, rows, opened),
    invalidate,
    recordKey,
    recordId: record.id,
    savedMessage,
    onDone: () => setEditing(false),
  });
  const existing =
    mutation.error instanceof ProblemError
      ? problemExistingId(mutation.error.problem)
      : null;
  const skew =
    mutation.error instanceof ProblemError &&
    isVersionSkew(mutation.error.problem);
  return (
    <>
      {/* Square in a header. A pencil is the one glyph every reader in this
          product's market already knows, and "Edit" beside it spends a
          header's width saying what the glyph said — on four record pages at
          once, which is why this is the one place it is written. The caller's
          own wording ("Edit deal", "Edit project") stays as the name, spoken
          and shown on hover, so the tooltip still says WHAT is being edited.

          Worded in a menu (`labelled`), where the same square would be the one
          row of the list that names nothing. */}
      {labelled ? (
        <Button
          small
          reasonId={disabledReasonId}
          onClick={() => setEditing(true)}
          data-testid="edit-record"
        >
          {label}
        </Button>
      ) : (
        <IconAction
          small
          label={label}
          icon={<PenLine size={15} aria-hidden="true" />}
          reasonId={disabledReasonId}
          onClick={() => setEditing(true)}
          testId="edit-record"
        />
      )}
      <EditRecordModal
        open={editing}
        onClose={() => setEditing(false)}
        title={label}
        notice={notice}
        fields={fields}
        record={record}
        pending={mutation.isPending}
        error={
          mutation.isError
            ? skew
              ? t("edit.versionSkew")
              : problemMessageOf(mutation.error, t)
            : null
        }
        existing={existing}
        resolveExisting={resolveExisting}
        onValuesChange={onValuesChange}
        onSubmit={(values, rows, opened) =>
          mutation.mutate({ values, rows: rows ?? {}, opened })
        }
      />
    </>
  );
}
