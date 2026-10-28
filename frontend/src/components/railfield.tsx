// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Pencil } from "lucide-react";
import { type ReactNode, useCallback, useRef, useState } from "react";
import { Button } from "../design-system/atoms";
import { IconAction } from "../design-system/iconaction";
import { InlineChoice } from "../design-system/inlinechoice";
import { InlineText } from "../design-system/inlinetext";
import { useLocale, useT } from "../i18n";
import type { EntityKind } from "../lib/entity";
import {
  ProblemError,
  problemExistingId,
  problemMessageOf,
} from "../lib/problem";
import type { Route } from "../lib/router";
import type { CreateField } from "./create";
import { EditRecordModal } from "./edit";
import { prefillFromRecord } from "./edit.prefill";
import type { FieldRecord, RecordFieldSave } from "./recordfieldtypes";
import { dateDisplay, selectOptions } from "./recordfieldvalues";
import { checkFieldValue, useRecordFieldWrite } from "./recordfieldwrite";
import { useUnsavedGuard } from "./unsaved";

/**
 * What a record's left column needs to change its fields. They are the same
 * fields, record and save the details tab hands `RecordFields`, so the two
 * places cannot write a field two ways.
 */
export type RailEdit = Readonly<{
  kind: EntityKind;
  record: FieldRecord;
  fields: readonly CreateField[];
  canEdit: boolean;
  save: RecordFieldSave;
  readOnlyFields?: Readonly<Record<string, string>>;
  resolveExisting?: (code: string, id: string) => Route;
  onValuesChange?: (values: Record<string, string>) => void;
}>;

type Props = Readonly<{
  edit: RailEdit;
  // The fields this row changes. One plain field edits in place; several, a
  // list of rows or a value drawn as a link open the edit form.
  keys: readonly string[];
  label: string;
  // The row's resting value, kept as the column draws it.
  reading: ReactNode;
  // The resting value as text, for a field that edits in place.
  text?: string;
  // The value is a link, so the edit is a glyph beside it, not the value.
  link?: boolean;
  beside?: ReactNode;
}>;

/** One left-column value that a reader with write access can change. */
export function RailField(props: Props) {
  const { edit, keys, reading, beside } = props;
  const fields = edit.fields.filter((field) => keys.includes(field.key));
  const locked = keys.some((key) => edit.readOnlyFields?.[key]);
  const [first] = fields;
  if (!first || !edit.canEdit || locked) {
    return (
      <>
        {reading}
        {beside}
      </>
    );
  }
  if (fields.length === 1 && !props.link && first.type === "select") {
    return <RailChoice {...props} field={first} />;
  }
  if (fields.length === 1 && !props.link && props.text !== undefined) {
    return <RailText {...props} field={first} />;
  }
  return <RailForm {...props} fields={fields} />;
}

type FieldProps = Props & Readonly<{ field: CreateField }>;

function useRailWrite(edit: RailEdit) {
  const opened = useRef(edit.record);
  const [editing, setEditing] = useState(false);
  const [dirty, setDirty] = useState(false);
  useUnsavedGuard(dirty, "details");
  const { mutation, pending } = useRecordFieldWrite(edit.kind, edit.record);
  // The baseline is the record as the editor opened on it. A refetch while it
  // is open must not move it, or the save overwrites a concurrent change.
  const onEditingChange = useCallback(
    (next: boolean) => {
      if (next && !editing) opened.current = edit.record;
      setEditing(next);
      if (!next) setDirty(false);
    },
    [editing, edit.record],
  );
  // A save elsewhere on the record pauses this field, never its own open editor.
  const canEdit = !pending || editing;
  return { opened, mutation, canEdit, setDirty, onEditingChange };
}

function RailText({ edit, field, label, text, beside }: FieldProps) {
  const t = useT();
  const { locale } = useLocale();
  const write = useRailWrite(edit);
  const stored = prefillFromRecord([field], edit.record)[field.key] ?? "";
  return (
    <>
      <InlineText
        label={label}
        value={stored}
        display={text || dateDisplay(field, stored, locale)}
        placeholder={t("field.unset")}
        multiline={field.type === "textarea"}
        type={
          field.type === "number" ||
          field.type === "date" ||
          field.type === "email"
            ? field.type
            : "text"
        }
        maxLength={field.maxLength}
        step={field.step ?? (field.type === "number" ? "any" : undefined)}
        canEdit={write.canEdit}
        onEditingChange={write.onEditingChange}
        onDirtyChange={write.setDirty}
        onSave={async (next) => {
          checkFieldValue(field, next, t);
          await write.mutation.mutateAsync({
            values: { [field.key]: next },
            rows: {},
            baseline: write.opened.current,
            save: edit.save,
          });
        }}
      />
      {beside}
    </>
  );
}

function RailChoice({ edit, field, label, reading, beside }: FieldProps) {
  const t = useT();
  const write = useRailWrite(edit);
  const values = prefillFromRecord([field], edit.record);
  const stored = values[field.key] ?? "";
  const options = selectOptions(field, values, t);
  return (
    <>
      <InlineChoice
        label={label}
        hideLabel
        value={stored}
        options={options}
        render={(value) =>
          // The stored value keeps the column's own reading, warnings included.
          stored && value === stored
            ? reading
            : (options.find((option) => option.value === value)?.label ?? value)
        }
        canEdit={write.canEdit}
        onEditingChange={write.onEditingChange}
        onDirtyChange={write.setDirty}
        onSave={async (next) => {
          checkFieldValue(field, next, t);
          await write.mutation.mutateAsync({
            values: { [field.key]: next },
            rows: {},
            baseline: write.opened.current,
            save: edit.save,
          });
        }}
      />
      {beside}
    </>
  );
}

function RailForm({
  edit,
  fields,
  label,
  reading,
  link,
  beside,
}: Props & Readonly<{ fields: readonly CreateField[] }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const { mutation, pending } = useRecordFieldWrite(edit.kind, edit.record);
  const change = t("inlineChoice.change", { field: label });
  const start = () => {
    mutation.reset();
    setOpen(true);
  };
  return (
    <>
      {link ? (
        <>
          {reading}
          <span className="recorddetails-glyph">
            <IconAction
              inline
              label={change}
              icon={<Pencil aria-hidden="true" />}
              disabled={pending}
              opensDialog
              onClick={start}
            />
          </span>
        </>
      ) : (
        <Button
          variant="link"
          className="inline-editable"
          aria-label={change}
          disabled={pending}
          onClick={start}
        >
          {reading}
        </Button>
      )}
      {beside}
      <EditRecordModal
        open={open}
        onClose={() => setOpen(false)}
        title={label}
        fields={[...fields]}
        record={edit.record}
        pending={mutation.isPending}
        error={mutation.error ? problemMessageOf(mutation.error, t) : null}
        existing={
          mutation.error instanceof ProblemError
            ? problemExistingId(mutation.error.problem)
            : null
        }
        resolveExisting={edit.resolveExisting}
        onValuesChange={edit.onValuesChange}
        onSubmit={(values, rows, opened) =>
          mutation.mutate(
            { values, rows: rows ?? {}, baseline: opened, save: edit.save },
            { onSuccess: () => setOpen(false) },
          )
        }
      />
    </>
  );
}
