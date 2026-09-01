// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../screens/story-utils";
import { AgentRail } from "./agentrail";
import type { GrantSpec } from "./mefixture";

// The section's states come from what the installation ANSWERS, so a story is
// a set of answers rather than a set of props. Each one below is a posture an
// installation is genuinely in — a deployment with no model bound, a mailbox
// whose token expired, an unlicensed workspace — because a story for a state
// no server can produce documents a screen nobody will see.
//
// It is mounted the way the shell mounts it: inside a rail's own foot
// (app/shell.tsx), because its geometry is the rail's own, to the pixel, and a
// story that floated it on blank canvas would be reviewing a different
// layout from the one that ships.

type Answers = Readonly<{
  aiState: "configured" | "unconfigured" | "development";
  connectorStatus: "connected" | "reauth_required";
  licenseState: "valid" | "absent" | "rejected";
  approvals: number;
  calls: readonly Readonly<{ task: string; minutesAgo: number }>[];
  /** What `/me/ai-activity` answers. The agent's own states come from here and
   *  from nowhere else, so a story for one is a story about this feed. */
  running?: readonly unknown[];
  recent?: readonly unknown[];
}>;

/** One occurrence as the wire spells it. */
function occurrence(over: Readonly<Record<string, unknown>>) {
  return {
    id: `run-${String(over.kind)}-${String(over.state)}`,
    kind: "morning_brief",
    state: "running",
    started_at: new Date(NOW - 4 * 60_000).toISOString(),
    ...over,
  };
}

// The two objects the section actually asks about: `license` gates the posture
// the orb reads (`useLicensePosture`) and `automation:update` gates the runtime
// row's `/ai/calls`. Granting exactly these rather than a blanket allow is what
// keeps a story named for an INSTALLATION posture from also quietly documenting
// an authority one — every story below is about what the installation answers,
// and `LicenceWithheld` is the one that is about the seat instead.
const OPERATOR: GrantSpec = {
  license: ["read"],
  automation: ["update"],
};

const NOW = Date.parse("2026-08-19T10:00:00Z");

function callRow(task: string, minutesAgo: number, index: number) {
  return {
    id: `call-${index}`,
    occurred_at: new Date(NOW - minutesAgo * 60_000).toISOString(),
    task,
    tier: "cheap_cloud",
    provider: "anthropic",
    model_id: "claude-sonnet-4-6",
    served_model: "claude-sonnet-4-6",
    calls_attempted: 1,
    tokens_in: 900,
    tokens_out: 120,
    reasoning_tokens: 0,
    cached_tokens: 0,
    latency_ms: 840,
    has_payload: false,
  };
}

function Rail({
  collapsed,
  children,
}: Readonly<{ collapsed?: boolean; children: ReactNode }>) {
  return (
    <nav className={collapsed ? "rail collapsed" : "rail expanded"}>
      <div className="grow" />
      <div className="railagent">{children}</div>
    </nav>
  );
}

function story(answers: Answers, collapsed = false, grants = OPERATOR) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(grants),
      "GET /assistant/profile": () =>
        jsonResponse({
          name: "Margince",
          kind: "ai",
          state: answers.aiState,
          inference_mode:
            answers.aiState === "development" ? "development" : "cloud",
          providers: answers.aiState === "configured" ? ["anthropic"] : [],
        }),
      "GET /installation/license": () =>
        jsonResponse({
          state: answers.licenseState,
          seats_used: 1,
          over_limit: false,
          checked_at: "2026-08-01T09:00:00Z",
        }),
      "GET /approvals": () =>
        jsonResponse({
          data: Array.from({ length: answers.approvals }, (_, index) => ({
            id: `approval-${index}`,
            status: "pending",
          })),
        }),
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              provider: "gmail",
              status: answers.connectorStatus,
              account_label: "ada@acme.test",
            },
          ],
        }),
      "GET /ai/calls": () =>
        jsonResponse({
          data: answers.calls.map((call, index) =>
            callRow(call.task, call.minutesAgo, index),
          ),
          tasks: [],
        }),
      "GET /ai/usage": () =>
        jsonResponse({
          days: [],
          budget: { monthly_tokens: 0, spent_tokens: 0, band: "normal" },
        }),
      "GET /me/ai-activity": () =>
        jsonResponse({
          running: answers.running ?? [],
          recent: answers.recent ?? [],
        }),
    });
    return (
      <StoryProviders>
        <Rail collapsed={collapsed}>
          <AgentRail route={{ screen: "companies" }} />
        </Rail>
      </StoryProviders>
    );
  };
}

const HEALTHY: Answers = {
  aiState: "configured",
  connectorStatus: "connected",
  licenseState: "valid",
  approvals: 0,
  calls: [
    { task: "growth_fit", minutesAgo: 12 },
    { task: "summarize", minutesAgo: 47 },
    { task: "brief_ranking", minutesAgo: 190 },
  ],
};

const meta: Meta<typeof AgentRail> = {
  title: "Shell/Agent rail",
  component: AgentRail,
};
export default meta;
type Story = StoryObj<typeof AgentRail>;

/** Idle: every source reachable, nothing waiting, a model bound, a valid licence. */
export const Idle: Story = { render: story(HEALTHY) };

/** Ingest: evidence arriving. Which half of the live vocabulary a run puts the
 *  orb in comes from the KIND of work (ai-activity-orb.ts), and a document being
 *  read is the plainest case of something coming in. */
export const Ingest: Story = {
  render: story({
    ...HEALTHY,
    running: [occurrence({ kind: "document_extract" })],
  }),
};

/** Working: the agent reasoning over evidence it already holds. Nothing this
 *  tab does reaches the orb, so the story is a run and not a pending write. */
export const Working: Story = {
  render: story({ ...HEALTHY, running: [occurrence({})] }),
};

/** Error: the overnight brief failed. It holds the orb until the panel has
 *  been opened, because a run that broke at four in the morning was seen by
 *  nobody. */
export const RunFailed: Story = {
  render: story({
    ...HEALTHY,
    recent: [occurrence({ state: "failed" })],
  }),
};

/** Warning: a live run past the lease its own source declared. The work may yet
 *  land, and there is nothing for the reader to do but know. */
export const RunStalled: Story = {
  render: story({ ...HEALTHY, running: [occurrence({ state: "stalled" })] }),
};

/** Warning: no licence bound. The orb goes amber and the line names the fault
 *  rather than raising it as a hard failure. */
export const Warning: Story = {
  render: story({ ...HEALTHY, licenseState: "absent" }),
};

/** Error: a mailbox the agent cannot reach — the token expired and capture is
 *  paused. Red means not connected, and the line says what is not connected. */
export const SourceUnreachable: Story = {
  render: story({ ...HEALTHY, connectorStatus: "reauth_required" }),
};

/** Error: no model bound at all — the one posture where nothing else on the
 *  section matters. */
export const NoModelConfigured: Story = {
  render: story({ ...HEALTHY, aiState: "unconfigured" }),
};

/** The development path: it answers, and every answer it gives is invented. */
export const DevelopmentModel: Story = {
  render: story({ ...HEALTHY, aiState: "development" }),
};

/** A fresh installation: a model is bound and nothing has run through it yet. */
export const NothingHasRunYet: Story = {
  render: story({ ...HEALTHY, calls: [] }),
};

/** The collapsed rail: the orb is the whole report at 64px, the words and the
 *  chevron gone. */
export const CollapsedRail: Story = {
  render: story(HEALTHY, true),
};

/** The panel open, showing the recap, the runtime row and the workspace
 *  section together — the detail the block itself has no room for. */
export const PanelOpen: Story = {
  render: story({ ...HEALTHY, approvals: 3 }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /expand/i }),
    );
  },
};

/**
 * A seat the licence is none of: the orb reports the installation's HEALTH and
 * stays neutral about its commercial standing.
 *
 * The distinction this story exists for is that a withheld licence must not
 * read as a fault. A rep's seat cannot see the entitlement, and an orb that
 * went amber about it on every screen they opened would be a permission
 * boundary drawn as a broken installation — so `useLicensePosture` answers
 * "nothing to report" rather than "something is wrong", and this is the story
 * that would fail if that ever changed.
 */
export const LicenceWithheld: Story = {
  render: story({ ...HEALTHY, licenseState: "absent" }, false, {
    automation: ["update"],
  }),
};
