// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

export const runningDataTask: components["schemas"]["DataTaskProgress"] = {
  task_id: "held-mail-correction",
  title: "Correct held mail",
  step: "classify",
  task_version: 1,
  status: "running",
  version: 4,
  batch_limit: 100,
  scanned: 200,
  changed: 12,
  batches: 2,
  estimate_capped: false,
  consecutive_failures: 0,
  missing_prerequisites: [],
  last_progress_at: "2026-01-15T09:30:00Z",
};
