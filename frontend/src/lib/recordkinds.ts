import { useQuery } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { unwrap } from "./problem";

// The record kinds installed extensions add and this seat may read. The
// generated types carry no enum for them, so the server answers instead. It
// changes only on deploy or a role change, so it is fetched once per session.
// A caller that sits on every screen passes `enabled` so the read waits for
// the screen that prints a kind's name.
export function useRecordKindDirectory(enabled = true) {
  return useQuery({
    queryKey: ["record-kinds"],
    queryFn: async () => unwrap(await api.GET("/record-kinds")),
    staleTime: Number.POSITIVE_INFINITY,
    enabled,
  });
}

// The headings a kind's declared fields carry, by column name, so a table over
// its rows says "Room" where the column is `room`. A column the kind does not
// declare has no entry and keeps its own name.
export function kindColumnLabels(
  name: string,
  directory: readonly {
    name: string;
    fields: readonly { name: string; label: string }[];
  }[],
): Readonly<Record<string, string>> {
  const fields = directory.find((entry) => entry.name === name)?.fields ?? [];
  return Object.fromEntries(fields.map((field) => [field.name, field.label]));
}

// What a screen prints for an extension's kind: the declared display name. A
// row can still name a kind whose unit was removed or that this seat may no
// longer read. That kind prints its own name, so a reader never sees `ext:<unit>:`.
export function recordKindLabel(
  name: string,
  directory: readonly { name: string; display_name: string }[],
): string {
  const declared = directory.find((entry) => entry.name === name);
  if (declared) {
    return declared.display_name;
  }
  const kind = name.split(":").at(-1) ?? name;
  return kind.replaceAll("_", " ");
}

type RecordKindOption = components["schemas"]["RecordKindOption"];

// The column a kind's records are named by: `name` where the kind declares one,
// else the first column of its natural key, else its first declared column.
function nameColumn(kind: RecordKindOption): string | undefined {
  return (
    kind.fields.find((field) => field.name === "name")?.name ??
    kind.natural_key[0] ??
    kind.fields[0]?.name
  );
}

function candidateOf(
  kind: RecordKindOption,
  column: string,
  row: Readonly<Record<string, unknown>>,
): RecordPickerCandidate {
  const detail = kind.natural_key
    .filter((key) => key !== column)
    .map((key) => row[key])
    .filter((value) => typeof value === "string" && value !== "")
    .join(" · ");
  return {
    id: String(row.id),
    name: String(row[column] ?? row.id),
    detail: detail || undefined,
  };
}

const PICK_LIMIT = 10;

async function previewKind(
  name: string,
  filter: Readonly<{ op: "contains" | "eq"; field: string; value: string }>,
) {
  return unwrap(
    await api.POST("/filters/preview", {
      body: { resource: name, filter, limit: PICK_LIMIT },
    }),
  ).rows;
}

/**
 * The search a picker over one extension kind runs: records whose name column
 * contains the typed text, through the same row-scoped preview the filter page
 * reads. A kind this reader may not read is absent from the directory and finds
 * nothing.
 */
export function searchKindRecords(
  name: string,
  directory: readonly RecordKindOption[],
): (term: string) => Promise<RecordPickerCandidate[]> {
  const kind = directory.find((entry) => entry.name === name);
  const column = kind && nameColumn(kind);
  return async (term) => {
    if (!kind || !column) {
      return [];
    }
    const rows = await previewKind(name, {
      field: column,
      op: "contains",
      value: term.trim(),
    });
    return rows.map((row) => candidateOf(kind, column, row));
  };
}

/** One record of an extension kind by id, or null when it cannot be read. */
export async function readKindRecord(
  name: string,
  directory: readonly RecordKindOption[],
  id: string,
): Promise<RecordPickerCandidate | null> {
  const kind = directory.find((entry) => entry.name === name);
  const column = kind && nameColumn(kind);
  if (!kind || !column) {
    return null;
  }
  const rows = await previewKind(name, {
    field: "id",
    op: "eq",
    value: id,
  });
  const [row] = rows;
  return row ? candidateOf(kind, column, row) : null;
}
