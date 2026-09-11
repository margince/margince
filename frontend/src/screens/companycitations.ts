// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHERE A CITED RECORD OPENS.
//
// The brief, the prepared answers and the suggestions all cite the same
// records, so they share one route: a second copy would drift and send one
// card's reader to the wrong screen. Its own file because it is that one route
// and nothing else — the screen that used to hold it is five times the file
// ceiling, and this is a concept that comes out whole.

import { navigate } from "../app/router";
import type { CitedRecord } from "./companyevidence";

// openCitation routes a cited record to its own screen. The brief, the
// prepared answers and the suggestions all cite the same records, so they
// share one route — a second copy would drift and send one card's reader to
// the wrong screen.
// A citation goes to one of two places. A deal or a person has a screen of its
// own; a fact or a profile field has no screen, but it does have a receipt —
// where the value came from and what could not be recorded about it — which is
// what the reader wanted when they clicked the chip.
export function citationOpensRecord(entityType: string): boolean {
  return entityType === "deal" || entityType === "person";
}

// An activity opens the MESSAGE, in the account page's own email drawer.
//
// The kinds a receipt can be written for. Narrowing HERE rather than asserting
// at the fetch is what keeps the modal's contract honest: a kind that grows a
// receipt upstream fails to compile until this decision learns about it.
export function citationHasReceipt(
  entityType: string,
): entityType is CitedRecord["entityType"] {
  return entityType === "fact" || entityType === "profile_field";
}

export function openCitation(entityType: string, entityId: string) {
  if (entityType === "deal") {
    navigate({ screen: "deals", id: entityId });
  }
  if (entityType === "person") {
    navigate({ screen: "contacts", id: entityId });
  }
}
