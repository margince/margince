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
import { type Handler, type MockRoute, page, reply } from "./server";

// The import screens' calls: the hub's list, and one CSV taken from upload to
// an approved run. Every write answers the run moved, so each screen of the
// wizard renders, and the run is remembered between calls. A test starts as the
// server answers it: 202 `validating`, and a later read finds it tested.

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

// The run the wizard works on. Staging a file resets it to the tested run.
// Every later write moves it, so a read answers what the last write did.
let current: ImportRun = tested;

function moveTo(next: ImportRun): Handler {
  return ({ json }) => {
    current = next;
    return json(current, 200);
  };
}

const validating: ImportRun = { ...tested, status: "validating" };

// The test runs in the worker, so the write answers 202 with the run
// `validating`. The first read after it still finds that, and the next one
// finds the run tested.
function startTest(version: number): Handler {
  return ({ json }) => {
    current = { ...validating, version };
    return json(current, 202);
  };
}

const readRun: Handler = ({ json }) => {
  const seen = current;
  if (seen.status === "validating") {
    current = { ...tested, version: seen.version };
  }
  return json(seen, 200);
};

export const importRoutes: readonly MockRoute[] = [
  { method: "GET", path: "/imports/format", handle: reply(formatFixture) },
  { method: "GET", path: "/imports", handle: reply(page(listed)) },
  { method: "POST", path: "/imports/sources", handle: reply(profile) },
  { method: "POST", path: "/imports", handle: startTest(tested.version) },
  {
    method: "GET",
    path: /^\/imports\/[^/]+\/report$/,
    handle: reply(report),
  },
  {
    method: "PATCH",
    path: RUN,
    handle: moveTo({ ...tested, status: "pending", version: 3 }),
  },
  {
    method: "POST",
    path: /^\/imports\/[^/]+\/validate$/,
    handle: startTest(4),
  },
  {
    method: "POST",
    path: /^\/imports\/[^/]+\/cancel$/,
    handle: moveTo({ ...draft, status: "cancelled", source_released: true }),
  },
  {
    method: "POST",
    path: /^\/imports\/[^/]+\/approve$/,
    handle: moveTo({ ...tested, status: "complete", source_released: true }),
  },
  {
    method: "POST",
    path: /^\/imports\/[^/]+\/undo$/,
    handle: moveTo({ ...tested, status: "undoing", source_released: true }),
  },
  { method: "GET", path: RUN, handle: readRun },
];
