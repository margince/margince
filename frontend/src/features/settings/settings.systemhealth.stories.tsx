// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { GrantSpec } from "../../testing/mefixture";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { SystemHealthPage } from "./settings.systemhealth";

// The whole page: a row of readings, and under it only the cards that have
// more to say than their answer.

const READ_AT = "2026-10-14T08:41:00Z";

const CLEAN = {
  "GET /embeddings/reindex/status": {
    status: "idle",
    reindex_needed: false,
    entities_pending: 0,
    configured_identity: "embed-v1",
    populated_identity: "embed-v1",
    updated_at: READ_AT,
  },
  "GET /admin/job-health": {
    generated_at: READ_AT,
    dead_window_hours: 24,
    kinds: [],
    recent_failures: [],
  },
  "GET /admin/extension-ingest-health": {
    generated_at: READ_AT,
    window_days: 7,
    units: [],
  },
  "GET /ai/provider-health": { providers: [], configured: ["anthropic"] },
  "GET /admin/capture-health": {
    generated_at: READ_AT,
    mailboxes: [],
    classifier: { pending: 0, unsure: 0, exhausted: 0 },
    held_meetings: { count: 0 },
    sweeps: [
      {
        sweep: "settled_thread_verdicts",
        cadence_seconds: 600,
        last_succeeded_at: READ_AT,
        last_run: {
          outcome: "ok",
          finished_at: READ_AT,
          processed: 2,
          cap_hit: false,
        },
      },
    ],
  },
  "GET /admin/automatic-work-health": {
    generated_at: READ_AT,
    days: 7,
    first_day: "2026-10-08",
    last_day: "2026-10-14",
    share_threshold: 0.1,
    small_sample_below: 10,
    kinds: [],
    colleagues: [],
    unattributed: 0,
  },
  "GET /admin/recovery-health": {
    generated_at: READ_AT,
    recovery_target_seconds: 14_400,
    data_loss_target_seconds: 3_600,
    last_drill: null,
  },
  "GET /installation/settings": { operations: defaultOperations },
};

// A provider out of credit: its slot turns red and its card comes back.
const PROVIDER_DOWN = {
  ...CLEAN,
  "GET /ai/provider-health": {
    configured: ["anthropic"],
    providers: [
      {
        provider: "anthropic",
        health: "out_of_credit",
        since: READ_AT,
        retry_after: "2026-10-14T09:41:00Z",
      },
    ],
  },
};

// A boot that fell back to the offline fake. The slot names it and links to
// AI models, where a key brings the bound models back.
const STAND_IN = {
  ...CLEAN,
  "GET /ai/provider-health": { providers: [], configured: ["fake"] },
};

// An intake report the server fails to read: its slot says so and its card
// stands under the row with a retry.
const INTAKE_FAILED: RouteMap = {
  "GET /admin/extension-ingest-health": () =>
    jsonResponse({ code: "internal", message: "Down." }, 500),
};

// Every report still on its way.
const IN_FLIGHT: RouteMap = Object.fromEntries(
  Object.keys(CLEAN).map((key) => [key, () => new Promise<Response>(() => {})]),
);

const OPERATOR: GrantSpec = {
  job_health: ["read"],
  ai_diagnostics: ["read"],
  embedding_reindex: ["read", "update"],
  installation_settings: ["read", "update"],
};

// A reader who may see the search index and providers, not the jobs.
const NO_JOBS: GrantSpec = {
  embedding_reindex: ["read"],
  ai_diagnostics: ["read"],
};

function page(
  routes: Record<string, unknown>,
  {
    grants = OPERATOR,
    handlers = {},
  }: { grants?: GrantSpec; handlers?: RouteMap } = {},
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(grants),
      ...Object.fromEntries(
        Object.entries(routes).map(([key, body]) => [
          key,
          () => jsonResponse(body),
        ]),
      ),
      ...handlers,
    });
    return (
      <StoryProviders>
        <div className="settings-stack">
          <SystemHealthPage />
        </div>
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/Governance/System health/Page",
  component: SystemHealthPage,
} satisfies Meta<typeof SystemHealthPage>;
export default meta;
type Story = StoryObj<typeof meta>;

export const AllClear: Story = {
  render: page(CLEAN),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Answering");
    await expect(
      canvas.queryByRole("heading", { name: "AI provider status" }),
    ).toBeNull();
  },
};

export const OneDegraded: Story = {
  render: page(PROVIDER_DOWN),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("heading", { name: "AI provider status" });
    await expect(canvas.getByText("Out of credit")).toBeInTheDocument();
  },
};

export const OneDegradedDark: Story = {
  globals: { theme: "dark" },
  render: page(PROVIDER_DOWN),
};

export const OneDegradedAtPhoneWidth: Story = {
  render: page(PROVIDER_DOWN),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const AnsweringFromTestProvider: Story = {
  render: page(STAND_IN),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Built-in test provider");
    await expect(
      canvas.getByRole("link", { description: "AI provider status" }),
    ).toHaveAttribute("href", "#/settings/models");
    await expect(
      canvas.getByText(
        "The bound models cannot be called. Add a key under AI models.",
      ),
    ).toBeInTheDocument();
  },
};

export const ReadFailed: Story = {
  render: page(CLEAN, { handlers: INTAKE_FAILED }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const row = await canvas.findByRole("region", { name: "Status" });
    await expect(
      await within(row).findByText("Could not check"),
    ).toBeInTheDocument();
    const card = await canvas.findByRole("region", {
      name: "Refused connector records",
    });
    await expect(
      await within(card).findByRole("button", { name: /retry/i }),
    ).toBeInTheDocument();
  },
};

export const JobsWithheld: Story = {
  render: page(CLEAN, { grants: NO_JOBS }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const row = await canvas.findByRole("region", { name: "Status" });
    await within(row).findByText("Up to date");
    await expect(within(row).queryByText("Background jobs")).toBeNull();
    await expect(
      await canvas.findByText(
        /background job health covers the whole installation/i,
      ),
    ).toBeInTheDocument();
  },
};

export const Checking: Story = {
  render: page({}, { handlers: IN_FLIGHT }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const row = await canvas.findByRole("region", { name: "Status" });
    await expect(
      (await within(row).findAllByText("Checking…")).length,
    ).toBeGreaterThan(0);
  },
};
