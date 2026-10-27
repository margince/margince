import { useQuery } from "@tanstack/react-query";

import { api } from "../api/client";
import { unwrap } from "./problem";

// The record kinds installed extensions add and this seat may read. The
// generated types carry no enum for them, so the server answers instead. It
// changes only on deploy or a role change, so it is fetched once per session.
export function useRecordKindDirectory() {
  return useQuery({
    queryKey: ["record-kinds"],
    queryFn: async () => unwrap(await api.GET("/record-kinds")),
    staleTime: Number.POSITIVE_INFINITY,
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
