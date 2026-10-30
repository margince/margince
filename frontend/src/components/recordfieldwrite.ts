// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useIsMutating,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import type { useT } from "../i18n";
import { derivedRecordKeys } from "../lib/activitykeys";
import type { EntityKind } from "../lib/entity";
import { ProblemError } from "../lib/problem";
import { invalidateRecord } from "../lib/recordwritekeys";
import type { CreateField, FormRows } from "./create";
import type { FieldRecord, RecordFieldSave } from "./recordfieldtypes";
import { rawRecord } from "./recordfieldtypes";

export type FieldWrite = Readonly<{
  values: Record<string, string>;
  rows: FormRows;
  baseline: FieldRecord;
  save: RecordFieldSave;
  extra?: Record<string, unknown>;
}>;

/**
 * The one write behind a record's in-place fields, wherever they are drawn:
 * the details tab's grid and the left column both save through it. Every
 * field of a record shares one key, so a save on one pauses the others.
 */
export function useRecordFieldWrite(kind: EntityKind, record: FieldRecord) {
  const qc = useQueryClient();
  const mutationKey = [`${kind}-edit`, record.id];
  const pending = useIsMutating({ mutationKey }) > 0;
  const mutation = useMutation({
    mutationKey,
    mutationFn: async ({ values, rows, baseline, save, extra }: FieldWrite) =>
      // An ordinary save carries no extra fields, only a refusal's resend does.
      extra
        ? save(values, rows, baseline, extra)
        : save(values, rows, baseline),
    onSuccess: async () => {
      await Promise.all([
        invalidateRecord(qc, kind, record.id, {
          anchor: rawRecord(record).is_anchor === true,
        }),
        qc.invalidateQueries({
          queryKey: [kind === "company" ? "companies" : `${kind}s`],
        }),
        ...derivedRecordKeys(kind, record.id).map((queryKey) =>
          qc.invalidateQueries({ queryKey }),
        ),
      ]);
    },
  });
  return { mutation, pending };
}

/** Refuses a typed value the field's own rules refuse, before any request. */
export function checkFieldValue(
  field: CreateField,
  next: string,
  t: ReturnType<typeof useT>,
): void {
  if (field.required && !next.trim())
    throw new ProblemError({ detail: t("record.fieldRequired") });
  const refusal = field.validate?.(next);
  if (refusal) throw new ProblemError({ detail: refusal });
}
