// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { defaultOperations } from "./operationsettings.fixtures";
import { SystemHealthPage } from "./settings.systemhealth";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

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

function page(routes: Record<string, unknown>) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        job_health: ["read"],
        ai_diagnostics: ["read"],
        embedding_reindex: ["read", "update"],
        installation_settings: ["read", "update"],
      }),
      ...Object.fromEntries(
        Object.entries(routes).map(([key, body]) => [
          key,
          () => jsonResponse(body),
        ]),
      ),
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

const meta: Meta<typeof SystemHealthPage> = {
  title: "Settings/Governance/System health/Page",
  component: SystemHealthPage,
};
export default meta;
type Story = StoryObj<typeof SystemHealthPage>;

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
