// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { attentionReport, HEALTH_AT } from "./systemhealth.fixtures";
import Diagnostics from "./systemhealthdiagnostics";

const meta = {
  title: "Settings/Governance/System health/Diagnostics",
  component: Diagnostics,
} satisfies Meta<typeof Diagnostics>;
export default meta;
type Story = StoryObj<typeof meta>;
export const RetainedRuns: Story = {
  args: { report: attentionReport },
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ job_health: ["read"] }),
      "GET /admin/job-health": () =>
        jsonResponse({
          generated_at: HEALTH_AT,
          dead_window_hours: 24,
          kinds: [],
          recent_failures: [],
        }),
      "GET /admin/job-health/history": () =>
        jsonResponse({
          observed_at: HEALTH_AT,
          cutoff: HEALTH_AT,
          window_start: HEALTH_AT,
          groups: [
            {
              kind: "capture_classify",
              state: "discarded",
              fleet_wide: true,
              count: 65,
              first_at: HEALTH_AT,
              latest_at: HEALTH_AT,
            },
          ],
          runs: [
            {
              kind: "capture_classify",
              state: "discarded",
              attempt: 3,
              max_attempts: 3,
              failed_at: HEALTH_AT,
              job_id: 4711,
              reason: "The job could not finish.",
            },
          ],
          has_more: true,
          next_cursor: "fixture",
          population_may_have_changed: false,
          coverage: "retained_only",
        }),
    });
    return (
      <StoryProviders>
        <Diagnostics report={attentionReport} />
      </StoryProviders>
    );
  },
};
export const Dark: Story = { ...RetainedRuns, globals: { theme: "dark" } };
