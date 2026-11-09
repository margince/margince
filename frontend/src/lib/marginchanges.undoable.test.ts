// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { type MarginceChangeBundle, shownUndoable } from "./marginchanges";

const bundle: MarginceChangeBundle = {
  deal_id: "d1",
  deal_name: "Deal",
  version: 3,
  audit_log_ids: ["a", "b", "c"],
  undoable_audit_log_ids: ["a", "c"],
  kinds: ["close_lost"],
  latest_at: "2026-11-08T08:00:00Z",
};

it("sends only shown changes the server still allows", () => {
  expect(shownUndoable(bundle, new Set(["a", "b"]))).toEqual(["a"]);
});

it("falls back to the listed changes from an older server", () => {
  const { undoable_audit_log_ids: _, ...older } = bundle;
  expect(shownUndoable(older, new Set(["b"]))).toEqual(["b"]);
});
