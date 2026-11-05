// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

// The operating values an installation nobody has tuned reads back, for the
// stories, tests and e2e mocks that answer the installation-settings read.
export const defaultOperations: components["schemas"]["OperationSettings"] = {
  agent_runner_interval_seconds: 30,
  webhook_retry_interval_seconds: 30,
  time_scan_interval_seconds: 3_600,
  close_date_sweep_interval_seconds: 86_400,
  follow_up_reconcile_interval_seconds: 86_400,
  retention_sweep_interval_seconds: 86_400,
  geocode_backfill_interval_seconds: 3_600,
  technical_backfill_interval_seconds: 21_600,
  gmail_watch_scan_interval_seconds: 21_600,
  graph_watch_scan_interval_seconds: 21_600,
  gmail_watch_renew_within_hours: 48,
  graph_watch_renew_within_hours: 24,
  send_rate_limit: 30,
  send_rate_window_seconds: 60,
  send_max_age_hours: 24,
};
