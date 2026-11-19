// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import { type Locale, useLocale, useT } from "../i18n";
import type { CfObject } from "../lib/customfields.logic";
import {
  ProblemError,
  problemExistingId,
  problemMessageOf,
} from "../lib/problem";
import type { CreateField } from "./create";
import { useSettledOpen } from "./create.dialog";
import { EditRecordModal } from "./edit";
import { prefillFromRecord } from "./edit.prefill";
import type { RailEdit } from "./railfield";
import { dateDisplay, groupValue } from "./recordfieldvalues";
import { useRecordFieldWrite } from "./recordfieldwrite";

/**
 * The edit form over some of a record's fields, saved through its one field write.
 * It opens once `catalog`'s fields have read, so a quick press still shows them.
 */
export function useFieldsDialog(
  edit: RailEdit,
  title: string,
  fields: readonly CreateField[],
  canOpen = true,
  catalog?: CfObject,
) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const shown = useSettledOpen(open, catalog);
  const { mutation, pending } = useRecordFieldWrite(edit.kind, edit.record);
  if (open && !canOpen) setOpen(false);
  const start = () => {
    mutation.reset();
    setOpen(true);
  };
  const element = (
    <EditRecordModal
      open={shown}
      onClose={() => setOpen(false)}
      title={title}
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
  );
  return { open, pending, start, element };
}

/** A row of a section whose change is a flow of its own, such as an owner handover. */
export type SectionFlow = Readonly<{
  label: string;
  reading?: ReactNode;
  open: () => void;
}>;

export type SectionEditOptions = Readonly<{
  /** The section's name, which titles its dialog. */
  label: string;
  /** The section's field and flow keys, in the order its rows read. */
  keys: readonly string[];
  /** The rows that open their own flow, by key. Leave out a flow the reader may not use. */
  flows?: Readonly<Record<string, SectionFlow>>;
  /** Fields shown but not changed here, by key; added to the edit's `readOnlyFields`. */
  readOnly?: Readonly<Record<string, SectionLock>>;
}>;

/** Why a field is locked, and its reading when the field's own text would misread it (money). */
export type SectionLock = Readonly<{ reason: string; reading?: ReactNode }>;

/** What a section's label line needs to offer its Edit. */
export type SectionEdit = Readonly<{ onEdit: () => void }>;

/**
 * A section's Edit: a dialog of only its fields, or its one flow opened directly.
 * `edit` is undefined when the reader may change nothing there.
 */
export function useSectionEdit(
  edit: RailEdit,
  section: SectionEditOptions,
): Readonly<{ edit: SectionEdit | undefined; element: ReactNode }> {
  const t = useT();
  const { locale } = useLocale();
  const fields = sectionFields(edit, section, t, locale);
  const changes = fields.filter((field) => !field.readOnly);
  const [only, ...others] = changes;
  const direct = others.length === 0 ? only?.flow : undefined;
  const dialog = useFieldsDialog(
    edit,
    section.label,
    fields,
    only !== undefined && !direct,
  );
  if (!only) return { edit: undefined, element: null };
  if (direct) return { edit: { onEdit: direct.open }, element: null };
  return { edit: { onEdit: dialog.start }, element: dialog.element };
}

function sectionFields(
  edit: RailEdit,
  { keys, flows = {}, readOnly = {} }: SectionEditOptions,
  t: ReturnType<typeof useT>,
  locale: Locale,
): CreateField[] {
  return keys.flatMap((key): CreateField[] => {
    const flow = flows[key];
    if (flow) return [{ key, labelText: flow.label, flow }];
    const field = edit.fields.find((candidate) => candidate.key === key);
    if (!field || !edit.canEdit) return [];
    const lock = readOnly[key];
    const reason = lock?.reason ?? edit.readOnlyFields?.[key];
    if (!reason) return [field];
    const stored = prefillFromRecord([field], edit.record)[key] ?? "";
    const reading =
      lock?.reading ??
      dateDisplay(field, stored, locale) ??
      groupValue([field], edit.record, t, locale, [], { [key]: reason });
    return [{ ...field, readOnly: { reading, reason } }];
  });
}
