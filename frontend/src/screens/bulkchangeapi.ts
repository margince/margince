// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The bulk dialog's two calls, for a change and for its undo. Kept apart from
// the dialog so the dialog file holds one concept: what the reader is shown.

import { api } from "../api/client";
import type { components } from "../api/schema";
import { requireVersion } from "../api/version";
import type { BulkChangeRequest, Translate } from "./bulkchange";
import { throwProblem } from "./common";

type BulkChangePreview = components["schemas"]["BulkChangePreview"];
type BulkChangeResult = components["schemas"]["BulkChangeResult"];

// The one request body both halves send. The preview and the execute must name
// the same selection, verb and owner, or the server refuses the confirm token.
function bulkBody(request: BulkChangeRequest) {
  return {
    record_type: request.recordType,
    verb: request.verb,
    items: request.rows.map((row) => ({
      id: row.id,
      version: requireVersion(row.version),
    })),
    owner_id: request.verb === "reassign_owner" ? request.ownerId : undefined,
    list_id: request.list?.id,
  };
}

// An undo previews the batch it names rather than a selection: the server
// knows which records the change altered and what they held before.
export async function previewBulkChange(
  request: BulkChangeRequest,
  t: Translate,
): Promise<BulkChangePreview> {
  const { data, error } = request.undoOf
    ? await api.POST("/bulk/{id}/undo/preview", {
        params: { path: { id: request.undoOf } },
      })
    : await api.POST("/bulk/preview", { body: bulkBody(request) });
  if (error) {
    throwProblem(error, t);
  }
  return data;
}

export type BulkRun = Readonly<{
  request: BulkChangeRequest;
  confirmToken?: string;
  idempotencyKey: string;
  t: Translate;
}>;

export async function executeBulkChange(
  run: BulkRun,
): Promise<BulkChangeResult> {
  const header = { "Idempotency-Key": run.idempotencyKey };
  const { data, error } = run.request.undoOf
    ? await api.POST("/bulk/{id}/undo", {
        params: { path: { id: run.request.undoOf }, header },
        body: { confirm_token: run.confirmToken },
      })
    : await api.POST("/bulk/execute", {
        params: { header },
        body: { ...bulkBody(run.request), confirm_token: run.confirmToken },
      });
  if (error) {
    throwProblem(error, run.t);
  }
  return data;
}
