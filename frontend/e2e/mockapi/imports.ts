// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  formatFixture,
  recentRunsFixture,
} from "../../src/screens/import.fixtures";
import type {
  ImportProfile,
  ImportReport,
  ImportRun,
} from "../../src/screens/importtypes";
import { type MockRoute, page, reply } from "./server";

// The import screens' calls: the hub's list, and one CSV taken from upload to
// an approved run. Every write answers the run moved, so each screen of the
// wizard renders; nothing is remembered between calls.

const draftId = "019ff-run-draft";

const draft: ImportRun = {
  id: draftId,
  connector: "csv",
  object: "lead",
  file_name: "leads.csv",
  status: "pending",
  checkpoint: 0,
  version: 1,
  source_released: false,
  source: "import_api",
  captured_by: "u-ada",
  created_at: "2026-10-13T09:00:00Z",
  updated_at: "2026-10-13T09:00:00Z",
};

const tested: ImportRun = {
  ...draft,
  status: "awaiting_approval",
  version: 2,
  parts: [{ file: "leads.csv", object: "lead", row_count: 2 }],
};

const profile: ImportProfile = {
  run_id: draftId,
  source_ref: "ws/import/leads",
  object: "lead",
  file_name: "leads.csv",
  rows_profiled: 2,
  row_count: 2,
  columns: [
    { header: "Email", fill_rate: 1, samples: ["ada@x.test"] },
    { header: "Full Name", fill_rate: 1, samples: ["Ada Lovelace"] },
  ],
  suggested_mapping: { Email: "email", "Full Name": "full_name" },
  targets: ["full_name", "email"],
};

const report: ImportReport = {
  run_id: draftId,
  status: "awaiting_approval",
  rows_read: 2,
  disposition: { created: 2, updated: 0, unchanged: 0, skipped: 0 },
  issues: [],
  source_key_used: "Email",
};

// A draft and a cancelled run sit in the list as the server answers it, so
// the hub is seen hiding them.
const listed: ImportRun[] = [
  { ...draft, id: "019ff-run-pending" },
  { ...draft, id: "019ff-run-cancelled", status: "cancelled" },
  ...recentRunsFixture,
];

const RUN = /^\/imports\/([^/]+)$/;

export const importRoutes: readonly MockRoute[] = [
  { method: "GET", path: "/imports/format", handle: reply(formatFixture) },
  { method: "GET", path: "/imports", handle: reply(page(listed)) },
  { method: "POST", path: "/imports/sources", handle: reply(profile) },
  { method: "POST", path: "/imports", handle: reply(tested) },
  {
    method: "GET",
    path: /^\/imports\/[^/]+\/report$/,
    handle: reply(report),
  },
  {
    method: "PATCH",
    path: RUN,
    handle: reply({ ...tested, status: "pending", version: 3 }),
  },
  {
    method: "POST",
    path: /^\/imports\/[^/]+\/validate$/,
    handle: reply({ ...tested, version: 4 }),
  },
  {
    method: "POST",
    path: /^\/imports\/[^/]+\/cancel$/,
    handle: reply({ ...draft, status: "cancelled", source_released: true }),
  },
  {
    method: "POST",
    path: /^\/imports\/[^/]+\/approve$/,
    handle: reply({ ...tested, status: "complete", source_released: true }),
  },
  {
    method: "POST",
    path: /^\/imports\/[^/]+\/undo$/,
    handle: reply({ ...tested, status: "undoing", source_released: true }),
  },
  { method: "GET", path: RUN, handle: reply(tested) },
];
