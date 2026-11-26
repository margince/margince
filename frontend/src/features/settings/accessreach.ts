// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ENTITY_KINDS, type EntityKind } from "../../lib/entity";

// What a seat may read and change among the record kinds, from the server's own
// answer: the object grants and the row scope. Row scope says whose records a
// write reaches, never whether the role holds a write. So a role with scope
// `all` and no write grant (Read-only) changes nothing. Every summary of a
// seat's access asks these two functions, so none can word a scope as a write.

type Grant = Readonly<{
  read?: boolean;
  create?: boolean;
  update?: boolean;
  delete?: boolean;
}>;

type Grants = Readonly<Record<string, Grant | undefined>> | undefined;

/** Whose records a seat may change, or `none` when it may change no record. */
export type ChangeReach = "none" | "own" | "team" | "all";

/** The record kinds these grants let a seat read, in the app's own order. */
export function readableKinds(objects: Grants): EntityKind[] {
  return ENTITY_KINDS.filter((kind) => objects?.[kind]?.read === true);
}

/**
 * Whose records these grants let a seat change. Only update and delete change
 * a record that exists, so a create-only grant changes none. A read seat
 * changes nothing whatever its role grants; `seat` is absent where no seat
 * exists yet (a role previewed before an invite). A scope this client does not
 * know narrows to `own`, as the contract tells a client to read a newer server.
 */
export function changeReach(
  objects: Grants,
  rowScope: string,
  seat?: string,
): ChangeReach {
  if (seat !== undefined && seat !== "full") {
    return "none";
  }
  const writes = ENTITY_KINDS.some((kind) => {
    const grant = objects?.[kind];
    return grant?.update || grant?.delete;
  });
  if (!writes) {
    return "none";
  }
  return rowScope === "all" || rowScope === "team" ? rowScope : "own";
}
