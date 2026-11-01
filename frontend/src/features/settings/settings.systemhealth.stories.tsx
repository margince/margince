// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { healthSessionRoutes } from "../../testing/systemhealth.session-fixtures";
import { HealthSessionPage } from "../../testing/systemhealth-session";
import { SystemHealthPage } from "./settings.systemhealth";
import {
  attentionReport,
  healthReport,
  overdueFinding,
} from "./systemhealth.fixtures";

function story(report: components["schemas"]["SystemHealth"]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        job_health: ["read"],
        embedding_reindex: ["read"],
        data_task: ["read"],
        ai_diagnostics: ["read"],
      }),
      "GET /admin/system-health": () => jsonResponse(report),
    });
    return (
      <StoryProviders>
        <SystemHealthPage />
      </StoryProviders>
    );
  };
}
const meta = {
  id: "settings-system-health",
  title: "Settings/Governance/System health/Overview",
  component: SystemHealthPage,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof SystemHealthPage>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Incomplete: Story = { render: story(healthReport) };
export const Attention: Story = { render: story(attentionReport) };
export const AttentionDark: Story = {
  render: story(attentionReport),
  globals: { theme: "dark" },
};
export const Narrow: Story = {
  render: story(attentionReport),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const SourceFailed: Story = {
  render: story({
    ...healthReport,
    assessment: "attention",
    action_count: 1,
    findings: [
      {
        ...overdueFinding,
        id: "jobs:read_failed",
        code: "read_failed",
        condition: "unknown",
        count: undefined,
        unit: undefined,
        job_kinds: undefined,
      },
    ],
    sources: healthReport.sources.map((source) =>
      source.id === "jobs"
        ? {
            ...source,
            availability: "failed",
            condition: "unknown",
            code: "read_failed",
          }
        : source,
    ),
  }),
};

function session(admin: boolean) {
  return () => {
    installFetchStub(healthSessionRoutes(admin));
    return (
      <StoryProviders>
        <HealthSessionPage />
      </StoryProviders>
    );
  };
}
export const AdminSession: Story = { render: session(true) };
export const CSOSession: Story = { render: session(true) };
export const LimitedRoleSession: Story = { render: session(false) };

export const PartialCapture: Story = {
  render: story({
    ...healthReport,
    sources: healthReport.sources.map((source) =>
      source.id === "capture"
        ? {
            ...source,
            availability: "unknown",
            condition: "clear",
            code: "capture_checked",
          }
        : source,
    ),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByText("All checks"));
    await expect(canvas.getByText(/Mail capture/)).toHaveTextContent(
      "No capture backlog detected in checked categories",
    );
  },
};
