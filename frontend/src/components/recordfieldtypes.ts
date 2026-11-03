import type { FormRows } from "./create";

export type FieldRecord = Record<string, unknown> & {
  id: string;
  version?: number;
};

export type FieldGroup = { label: string; keys: string[] };

// Conflict comparisons use the API values from before form projection.
export function rawRecord(opened: FieldRecord): FieldRecord {
  const original = opened.original;
  if (
    original &&
    typeof original === "object" &&
    "id" in original &&
    typeof original.id === "string"
  )
    return { ...original, id: original.id };
  return opened;
}

export type RecordFieldSave = (
  values: Record<string, string>,
  rows: FormRows,
  opened: FieldRecord,
  // A refusal's verb adds body fields when it resends the same edit.
  extra?: Record<string, unknown>,
) => Promise<unknown>;
