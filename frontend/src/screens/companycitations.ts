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

// Opens the kinds `citationOpensRecord` names; a fact or a profile field has no
// screen, and opens the page's receipt drawer through `onOpenReceipt` instead.
export function openCitation(entityType: string, entityId: string) {
  if (entityType === "deal") {
    navigate({ screen: "deals", id: entityId });
  }
  if (entityType === "contact") {
    navigate({ screen: "contacts", id: entityId });
  }
}
