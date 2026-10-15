// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { meFixture } from "../../testing/mefixture";
import {
  emptyPage,
  jsonResponse,
  type RouteMap,
} from "../../testing/story-utils";

type Contact = components["schemas"]["Contact"];
type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];

// An archived contact as GET /contacts/{id} answers it: no addresses, which the
// server leaves off an archived row. The story and the suite share it.
export const archivedContact: Contact = {
  id: "p-1",
  full_name: "Dana Buyer",
  owner_id: "u-1",
  visibility: "workspace",
  writable: false,
  version: 8,
  archived_at: "2026-08-14T09:00:00Z",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-14T09:00:00Z",
};

export const archiveEntry: AuditHistoryEntry = {
  id: "h-archive",
  actor_type: "human",
  actor_id: "u-1",
  actor_name: "Demo Admin",
  action: "archive",
  occurred_at: "2026-08-14T09:00:00Z",
  summary: "Demo Admin archived the record",
  undoable: { undoable: true },
};

// The 404 the server answers the 360, the brief and the guard with for an
// archived row.
export const refusedRead = () =>
  jsonResponse(
    { status: 404, code: "not_found", title: "Not Found", detail: "not found" },
    404,
  );

export function archivedContactRoutes(): RouteMap {
  return {
    "GET /me": () =>
      jsonResponse(
        meFixture({
          allow: {
            contact: ["read", "update"],
            activity: ["create"],
            lead: ["create", "read"],
          },
          settingsAvailability: { lists: true },
        }),
      ),
    "GET /contacts/p-1": () => jsonResponse(archivedContact),
    "GET /contacts/p-1/360": refusedRead,
    "GET /contacts/p-1/brief": refusedRead,
    "GET /contacts/p-1/consent/guard": refusedRead,
    "GET /records/contact/p-1/history": () =>
      jsonResponse({ data: [archiveEntry], page: emptyPage.page }),
    "GET /field-history": () => jsonResponse(emptyPage),
  };
}
