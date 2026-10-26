// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ENTITY, type EntityKind } from "../lib/entity";
import { navigate } from "../lib/router";

// The cited kinds that open a screen, which the record registry routes. Not
// `activity`, which opens its message per row, nor `company`, usually this page.
export const CITED_RECORD_KINDS = [
  "deal",
  "contact",
  "lead",
] as const satisfies readonly EntityKind[];

const CITED_RECORDS: ReadonlySet<string> = new Set(CITED_RECORD_KINDS);

export function citationOpensRecord(
  kind: string,
): kind is (typeof CITED_RECORD_KINDS)[number] {
  return CITED_RECORDS.has(kind);
}

export function openCitation(entityType: string, entityId: string) {
  if (citationOpensRecord(entityType)) {
    navigate(ENTITY[entityType].route(entityId));
  }
}
