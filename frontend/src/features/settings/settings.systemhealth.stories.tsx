// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  type HealthSessionAccess,
  healthSenderReviewRoutes,
  healthSessionRoutes,
} from "../../testing/systemhealth.session-fixtures";
import { HealthSessionPage } from "../../testing/systemhealth-session";
import { SystemHealthPage } from "./settings.systemhealth";
import {
  attentionReport,
  healthReport,
  overdueFinding,
  stoppedMailReport,
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
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByRole("heading", { name: "Attention needed" }),
    ).resolves.toBeVisible();
  },
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

function session(access: HealthSessionAccess) {
  return () => {
    installFetchStub(healthSessionRoutes(access));
    return (
      <StoryProviders>
        <HealthSessionPage />
      </StoryProviders>
    );
  };
}
export const AdminSession: Story = { render: session("configure") };
export const CSOSession: Story = { render: session("diagnose") };
export const LimitedRoleSession: Story = { render: session("denied") };

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
      "No capture action needed in checked categories",
    );
  },
};

export const MailDecisions: Story = {
  render: story({
    ...attentionReport,
    action_count: 2,
    sources: healthReport.sources.map((source) =>
      source.id === "capture"
        ? {
            ...source,
            condition: "waiting_human",
            code: "capture_uncertain",
          }
        : source,
    ),
    findings: ["capture_uncertain", "capture_thread_uncertain"].map(
      (code, index) => ({
        ...overdueFinding,
        id: `capture:${code}`,
        source: "capture",
        code,
        condition: "waiting_human",
        responsible: "mailbox_owner",
        count: index + 1,
        unit: index === 0 ? "records" : "threads",
        job_kinds: undefined,
      }),
    ),
  }),
};
export const MailDecisionsDark: Story = {
  ...MailDecisions,
  globals: { theme: "dark" },
};

export const ResolutionOverview: Story = {
  render: () => {
    const reviews = healthSenderReviewRoutes();
    const senderFinding: components["schemas"]["HealthFinding"] = {
      ...overdueFinding,
      id: "capture:capture_uncertain",
      source: "capture",
      code: "capture_uncertain",
      condition: "waiting_human",
      urgency: 1,
      responsible: "mailbox_owner",
      count: 2,
      unit: "records",
      job_kinds: undefined,
    };
    installFetchStub({
      ...healthSessionRoutes("configure"),
      ...reviews.routes,
      "GET /admin/system-health": () =>
        jsonResponse({
          ...stoppedMailReport,
          action_count: 2,
          findings: [
            ...stoppedMailReport.findings,
            { ...senderFinding, count: reviews.remaining() + 1 },
          ],
          sources: stoppedMailReport.sources.map((source) =>
            source.id === "capture"
              ? {
                  ...source,
                  condition: "waiting_human",
                  code: "capture_uncertain",
                }
              : source,
          ),
        }),
      "GET /admin/capture-health": () =>
        jsonResponse({
          generated_at: stoppedMailReport.generated_at,
          mailboxes: [],
          sweeps: [],
          connection_concerns: [],
          classifier: {
            pending: 0,
            unsure: reviews.remaining() + 1,
            exhausted: 0,
          },
          held_meetings: { count: 0 },
          owner_reviews: [
            {
              user_id: "00000000-0000-4000-8000-000000000001",
              display_name: "Alex",
              sender_decisions: reviews.remaining(),
              thread_decisions: 0,
            },
            {
              user_id: "00000000-0000-4000-8000-000000000002",
              display_name: "Maya",
              sender_decisions: 1,
              thread_decisions: 0,
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <HealthSessionPage />
      </StoryProviders>
    );
  },
};
export const ResolutionOverviewDark: Story = {
  ...ResolutionOverview,
  globals: { theme: "dark" },
};
export const ResolutionOverviewPhone: Story = {
  ...ResolutionOverview,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const SenderDecisionJourney: Story = {
  ...ResolutionOverview,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Review mailbox decisions" }),
    );
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Review my choices" }),
    );
    await waitFor(() =>
      expect(canvas.getByText("Add Alex Buyer to Contacts")).toBeVisible(),
    );
    await userEvent.click(
      await canvas.findByRole("button", { name: "Decide" }),
    );
    await userEvent.click(await body.findByRole("button", { name: "Approve" }));
    await waitFor(() =>
      expect(
        canvas.queryByText("Add Alex Buyer to Contacts"),
      ).not.toBeInTheDocument(),
    );
  },
};
