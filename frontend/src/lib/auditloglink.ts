// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { routeHash } from "./router";
import { settingsHref } from "./settingsrouting";
import { hashWithParams } from "./urlstate";

export const AUDIT_ENTITY_TYPE_PARAM = "entity_type";
export const AUDIT_ENTITY_ID_PARAM = "entity_id";

/** The audit log, opened on the history of one record. */
export function auditHistoryHref(entityType: string, id: string): string {
  return hashWithParams(
    routeHash(settingsHref("audit")),
    new Map([
      [AUDIT_ENTITY_TYPE_PARAM, entityType],
      [AUDIT_ENTITY_ID_PARAM, id],
    ]),
  );
}
