// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { aiAdminFixture } from "../ai-admin-fixture";
import {
  aiCallStats,
  aiRouting,
  aiUsage,
  installationLicense,
} from "./fixtures";
import type { Handler } from "./server";

// Neither a refusal nor an absence verifies a grant, so both drop the seat
// cap the way the server does.
export const license: Handler = ({ json }, { options }) => {
  if (options.license === "absent" || options.license === "rejected") {
    return json({
      state: options.license,
      seats_used: installationLicense.seats_used,
      over_limit: false,
      checked_at: installationLicense.checked_at,
    });
  }
  return json(installationLicense);
};

// Relative to now, because the rail ages a run by its start.
export const runningActivity: Handler = ({ json }) =>
  json({
    as_of: new Date().toISOString(),
    running: [
      {
        id: "019f7e65-fbf7-7114-b114-40af4af63a01",
        kind: "morning_brief",
        state: "running",
        started_at: new Date(Date.now() - 60_000).toISOString(),
      },
    ],
    recent: [],
    faults: [],
  });

export const aiStatus: Handler = ({ json }) =>
  json(aiAdminFixture(aiRouting.tiers, aiUsage.budget));

export const aiBudget: Handler = ({ json }) =>
  json(aiAdminFixture(aiRouting.tiers, aiUsage.budget).budget);

export const callStats: Handler = ({ url, json }) =>
  json(aiCallStats(url.searchParams.get("group")));

export const callStatsFlow: Handler = ({ url, json }) =>
  json({
    task: url.searchParams.get("task"),
    window: "7d",
    total: 0,
    unanswered: 0,
    steps: [],
  });
