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
  /** What `/ai/calls` answers. It feeds the runtime row's served model and
   *  nothing else — the recap is the feed's, below — so one row is the whole
   *  fixture and an empty list is the installation that has never called. */
  calls: readonly Readonly<{ task: string; minutesAgo: number }>[];
  /** What `/me/ai-activity` answers. The agent's own states come from here and
   *  from nowhere else, so a story for one is a story about this feed. */
  running?: readonly unknown[];
  recent?: readonly unknown[];
  /** A mailbox import in flight, as the connections read reports it. */
  importing?: Readonly<{ scanned: number; estimated: number | null }>;
  /** What the month cost, in minor units, when anything in it was priced.
   *  Absent is the ordinary case and a real one: a month where nothing carried
   *  a price prints no figure at all, which is a different statement from
   *  "this cost nothing". */
  pricedMinor?: number;
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

/** A settled occurrence, as the recap reads them: finished, and newest first. */
function settled(minutesAgo: number, over: Readonly<Record<string, unknown>>) {
  return occurrence({
    state: "done",
    started_at: new Date(NOW - (minutesAgo + 1) * 60_000).toISOString(),
    finished_at: new Date(NOW - minutesAgo * 60_000).toISOString(),
    ...over,
  });
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

// The month's spend is served on `ai_diagnostics:read` and is the
// administrator's figure, so the story that shows it holds that grant on top of
// the two above. Kept apart from OPERATOR because every other story here is
// about an installation posture rather than about what this seat may read.
const SPEND_READER: GrantSpec = { ...OPERATOR, ai_diagnostics: ["read"] };

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

// The three feet the block has to look right in, named by the class the shell
// puts on the rail (app/shell.tsx) — the stylesheet keys the reduction off that
// state, so a story that dressed itself would be showing a rail this app never
// renders.
type RailState = "expanded" | "collapsed" | "leveled";

function Rail({
  state,
  children,
}: Readonly<{ state: RailState; children: ReactNode }>) {
  return (
    <nav className={`rail ${state}`}>
      <div className="grow" />
      <div className="railagent">{children}</div>
    </nav>
  );
}

function story(
  answers: Answers,
  rail: RailState = "expanded",
  grants = OPERATOR,
) {
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
              backfill: answers.importing
                ? {
                    state: "running",
                    estimated_messages: answers.importing.estimated,
                    counts: { messages_scanned: answers.importing.scanned },
                  }
                : { state: "none" },
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
          // One priced task and one the server could not price, which is the
          // shape the month actually arrives in: the total is the priced lines
          // and the unpriced one adds nothing to it.
          days:
            answers.pricedMinor === undefined
              ? []
              : [
                  {
                    date: "2026-08-01",
                    tasks: [
                      {
                        task: "enrich",
                        tier: "cheap_cloud",
                        calls: 2,
                        tokens_in: 100,
                        tokens_out: 40,
                        cost_est_minor: answers.pricedMinor,
                      },
                      {
                        task: "summarize",
                        tier: "cheap_cloud",
                        calls: 1,
                        tokens_in: 30,
                        tokens_out: 10,
                      },
                    ],
                  },
                ],
          budget: {
            monthly_tokens: 0,
            spent_tokens: 0,
            band: "normal",
            currency: "USD",
          },
        }),
      "GET /me/ai-activity": () =>
        jsonResponse({
          running: answers.running ?? [],
          recent: answers.recent ?? [],
        }),
    });
    return (
      <StoryProviders>
        <Rail state={rail}>
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
  calls: [{ task: "summarize", minutesAgo: 12 }],
};

/**
 * A day of finished work, as the panel's recap reads it.
 *
 * Two of the three name the record they were about, which is the half of a
 * recap row a reader can act on — the name is the way back to the account. The
 * third names none, because most occurrences are about no single record and the
 * row has to read well without one.
 *
 * It is the PanelOpen story's alone rather than HEALTHY's: `recent` also feeds
 * the card's resting rotation, so putting it in the shared fixture would put a
 * finished run into the line of every story in this file.
 */
const A_DAYS_WORK = [
  settled(12, {
    kind: "site_read",
    subject_label: "Acme GmbH",
    subject_type: "company",
    subject_id: "019f7e65-0000-7000-8000-0000000000b2",
  }),
  settled(47, {
    kind: "summarize",
    subject_label: "Ana Roth",
    subject_type: "contact",
    subject_id: "019f7e65-0000-7000-8000-0000000000b3",
  }),
  settled(190, { kind: "morning_brief" }),
];

const meta: Meta<typeof AgentRail> = {
  title: "Shell/Agent rail",
  component: AgentRail,
};
export default meta;
type Story = StoryObj<typeof AgentRail>;

/** Idle: every source reachable, nothing waiting, a model bound, a valid licence.
 *
 *  It is also the block's last line with no figure on it: this seat holds no
 *  `ai_diagnostics:read`, so the row carries the chevron alone. The row is what
 *  makes that state legible — the disclosure keeps its place instead of moving
 *  up into the corner when there is nothing to spend beside it. */
export const Idle: Story = { render: story(HEALTHY) };

/** The month's spend, on the block's last line with the chevron that opens the
 *  panel behind it: one figure, the scope it was spent in, and the disclosure,
 *  all on one baseline. The seat is an administrator holding
 *  `ai_diagnostics:read`, which is the only seat the server serves it to. */
export const SpendReported: Story = {
  render: story({ ...HEALTHY, pricedMinor: 1_240 }, "expanded", SPEND_READER),
};

/** The same figure in dark, and it is the money that needs looking at rather
 *  than the block: the figure and the scope beside it are both `--textMeta` at
 *  eyebrow size, on a rail whose ground is the translucent `--pane` over the
 *  page's glow — so the smallest type on the surface stands on a composite that
 *  each theme mixes differently. */
export const SpendReportedDark: Story = {
  globals: { theme: "dark" },
  render: story({ ...HEALTHY, pricedMinor: 1_240 }, "expanded", SPEND_READER),
};

/** Ingest: evidence arriving. Which half of the live vocabulary a run puts the
 *  orb in comes from the KIND of work (ai-activity-orb.ts), and a document being
 *  read is the plainest case of something coming in. */
export const Ingest: Story = {
  render: story({
    ...HEALTHY,
    running: [occurrence({ kind: "document_extract" })],
  }),
};

/** Ingest, from a mailbox import: the one long run the feed cannot carry,
 *  read off the connections list instead. The ring is the import's share and
 *  the line says so with it. */
export const ImportingMail: Story = {
  render: story({
    ...HEALTHY,
    importing: { scanned: 1_204, estimated: 2_900 },
  }),
};

/** The same import with no preview behind it: the orb ingests, the line names
 *  the import, and no ring is drawn at a guessed share. */
export const ImportingMailUnpreviewed: Story = {
  render: story({ ...HEALTHY, importing: { scanned: 37, estimated: null } }),
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

/**
 * The line changing, which is the only motion this block has of its own.
 *
 * Two true readings and a queue, so the resting rotation has more than one
 * thing to say and swaps every `IDLE_HOLD_MS`. Watch the slot rather than the
 * orb: the outgoing sentence fades out under the incoming one over a single
 * `--dur-enter`, the two-line room holds still, and nothing under it moves.
 *
 * Every story on this page plays the same crossfade once at mount — the
 * section's own reads are named ones, so the ticker says "Checking what needs
 * you" and hands the slot back when the read settles (`agentrail-ticker.ts`).
 * This is the one that keeps doing it.
 */
export const IdleRotation: Story = {
  render: story({ ...HEALTHY, aiState: "development", approvals: 3 }),
};

/** The rotation in dark, where the crossfade is the thing to watch: both layers
 *  are `--textPrimary` and the outgoing one is drawn OVER the incoming one, so
 *  a fade whose midpoint reads as two sentences on light can read as one
 *  smeared sentence on a ground with less contrast to spend. */
export const IdleRotationDark: Story = {
  globals: { theme: "dark" },
  render: story({ ...HEALTHY, aiState: "development", approvals: 3 }),
};

/** A fresh installation: a model is bound and nothing has run through it yet. */
export const NothingHasRunYet: Story = {
  render: story({ ...HEALTHY, calls: [] }),
};

/** The collapsed rail: the orb is the whole report at 56px, the words and the
 *  chevron gone. */
export const CollapsedRail: Story = {
  render: story(HEALTHY, "collapsed"),
};

/** The rail showing one section's entries: the agent keeps its foot, at the size
 *  that says it is still reporting without leading a list it is not about. */
export const LeveledRail: Story = {
  render: story(HEALTHY, "leveled"),
};

/**
 * The panel open, showing the recap, the runtime row and the workspace section
 * together — the detail the block itself has no room for, with the panel's one
 * control in its foot.
 *
 * That switch governs the lit window edge, and it is deliberately the only
 * story it gets: the preference is one per browser (`agent-edge-preference.ts`),
 * so a second story that opened with it off would set it off for every other
 * story in the catalog after it. Its two appearances are `Switch`'s own.
 */
export const PanelOpen: Story = {
  render: story({ ...HEALTHY, approvals: 3, recent: A_DAYS_WORK }),
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
  render: story({ ...HEALTHY, licenseState: "absent" }, "expanded", {
    automation: ["update"],
  }),
};
