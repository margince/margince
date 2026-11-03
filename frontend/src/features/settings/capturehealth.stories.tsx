// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { CaptureHealthCard } from "./capturehealth";

// Whether mail capture keeps up, for an administrator who may not see the mail.
// The states worth a picture: every pass keeping up, passes that need
// attention for each reason the card names, a fresh installation where nothing
// has run yet, and a reader without the grant.

type Health = components["schemas"]["CaptureHealth"];

const meta = {
  title: "Settings/Governance/System health/Mail capture checks",
  component: CaptureHealthCard,
} satisfies Meta<typeof CaptureHealthCard>;
export default meta;
type Story = StoryObj<typeof meta>;

// The seeded matrix gives `job_health:read` to admin and ops only, so a story's
// grant follows its roles and Withheld really draws the withheld state.
const READS_JOB_HEALTH = ["admin", "ops"];

function grantFor(roles: string[]): GrantSpec {
  return roles.some((role) => READS_JOB_HEALTH.includes(role))
    ? { job_health: ["read"] }
    : {};
}

function story(health: Health, roles: string[] = ["admin"]) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ roles, allow: grantFor(roles) })),
      "GET /admin/capture-health": () => jsonResponse(health),
    });
    return (
      <StoryProviders>
        <CaptureHealthCard />
      </StoryProviders>
    );
  };
}

const GENERATED = "2026-09-17T09:30:00Z";

const run = (
  outcome: "ok" | "partial" | "failed" | "skipped",
  extra: Partial<components["schemas"]["CaptureSweepRun"]> = {},
) => ({
  outcome,
  finished_at: "2026-09-17T03:00:00Z",
  processed: outcome === "partial" ? 200 : 4,
  cap_hit: outcome === "partial",
  ...extra,
});

const KEEPING_UP: Health = {
  connection_concerns: [],
  generated_at: GENERATED,
  mailboxes: [],
  classifier: { pending: 0, unsure: 0, exhausted: 0 },
  held_meetings: { count: 0 },
  sweeps: [
    {
      sweep: "settled_thread_verdicts",
      cadence_seconds: 600,
      last_succeeded_at: "2026-09-17T09:20:00Z",
      last_run: run("ok", { finished_at: "2026-09-17T09:20:00Z" }),
    },
    {
      sweep: "stranded_contacts",
      cadence_seconds: 86_400,
      last_succeeded_at: "2026-09-17T03:00:00Z",
      last_run: run("ok"),
    },
    {
      sweep: "filed_meeting_holds",
      cadence_seconds: 86_400,
      last_succeeded_at: "2026-09-17T03:00:00Z",
      last_run: run("ok"),
    },
  ],
};

// Nothing waits anywhere and every pass ran on time. The count says no mailbox
// waits, so neither the mailbox table nor the state column is drawn.
export const KeepingUp: Story = {
  render: story(KEEPING_UP),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Mailboxes with something waiting");
    await expect(
      canvas.queryByRole("heading", { name: "Mailboxes" }),
    ).toBeNull();
    await expect(
      canvas.queryByRole("columnheader", { name: "State" }),
    ).toBeNull();
  },
};

// One pass per warning: a failure with its class, a pass skipped behind a
// failed stage, and a pass stopped at its bound. Two mailboxes carry backlogs.
export const NeedsAttention: Story = {
  render: story({
    connection_concerns: [],
    generated_at: GENERATED,
    mailboxes: [
      {
        user_id: "0190b7a2-0000-7000-8000-000000000001",
        display_name: "Ana Rep",
        contacts_awaiting_decision: 14,
        oldest_contact_age_seconds: 9 * 86_400,
        threads_awaiting_verdict: 3,
        oldest_thread_age_seconds: 5 * 3_600,
      },
      {
        user_id: "0190b7a2-0000-7000-8000-000000000002",
        display_name: "Ben Manager",
        contacts_awaiting_decision: 0,
        threads_awaiting_verdict: 1,
        oldest_thread_age_seconds: 40 * 60,
      },
    ],
    classifier: {
      pending: 12,
      unsure: 30,
      exhausted: 2,
      oldest_pending_age_seconds: 2 * 86_400,
    },
    held_meetings: { count: 231, oldest_age_seconds: 3 * 86_400 },
    sweeps: [
      {
        sweep: "settled_thread_verdicts",
        cadence_seconds: 600,
        last_succeeded_at: "2026-09-17T07:10:00Z",
        last_run: run("skipped", {
          finished_at: "2026-09-17T09:20:00Z",
          processed: 0,
        }),
      },
      {
        sweep: "stranded_contacts",
        cadence_seconds: 86_400,
        last_succeeded_at: "2026-09-14T03:00:00Z",
        last_run: run("failed", { error_class: "write_conflict" }),
      },
      {
        sweep: "filed_meeting_holds",
        cadence_seconds: 86_400,
        last_succeeded_at: "2026-09-17T03:00:00Z",
        last_run: run("partial"),
      },
    ],
  }),
};

// A fresh installation: no pass has written a receipt yet. Said as "never
// run" rather than drawn as keeping up.
export const NeverRun: Story = {
  render: story({
    ...KEEPING_UP,
    sweeps: KEEPING_UP.sweeps.map(({ sweep, cadence_seconds }) => ({
      sweep,
      cadence_seconds,
    })),
  }),
};

// A reader without the grant. The card keeps its place and says the reading is
// not theirs; an absent card would read as "capture keeps up".
export const Withheld: Story = { render: story(KEEPING_UP, ["rep"]) };

export const NeedsAttentionAtPhoneWidth: Story = {
  ...NeedsAttention,
  tags: ["uat-phone"],
};

export const NeedsAttentionDark: Story = {
  ...NeedsAttention,
  globals: { theme: "dark" },
};
