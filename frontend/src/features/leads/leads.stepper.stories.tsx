// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { LeadStepper } from "./leads.stepper";

type Lead = components["schemas"]["Lead"];

// The ladder a lead climbs, drawn under the page's header: New → Contacted →
// Engaged, then the two closures set apart from them.
//
// The states worth seeing are the ones where the ladder REFUSES, because that
// is where the component earns its keep: a closed lead and a mirrored one both
// draw every step and let a reader press none of them, each saying why on the
// control rather than beside it.

function lead(overrides: Partial<Lead> = {}): Lead {
  return {
    id: "l-1",
    full_name: "Jonas Petersen",
    email: "jonas@nordwind.example",
    company_name: "Nordwind Logistik",
    status: "contacted",
    score: 72,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: "2026-08-20T09:00:00Z",
    updated_at: "2026-08-24T11:00:00Z",
    ...overrides,
  };
}

function ladder(props: {
  lead?: Partial<Lead>;
  pending?: boolean;
  closed?: boolean;
  bars?: Partial<Record<"new" | "contacted" | "engaged", string>>;
}) {
  return () => {
    installFetchStub({});
    return (
      <StoryProviders>
        {/* The page's one sentence, named by every refused rung. */}
        {props.closed && (
          <p id="lead-closed">
            This lead is closed. It takes no further changes.
          </p>
        )}
        <LeadStepper
          lead={lead(props.lead)}
          pending={props.pending ?? false}
          reasonId={props.closed ? "lead-closed" : undefined}
          variant={props.bars ? "bars" : undefined}
          dates={props.bars}
          onStep={() => undefined}
          onQualify={() => undefined}
          onDisqualify={() => undefined}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof LeadStepper> = {
  title: "Records/Leads/Ladder",
  component: LeadStepper,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof LeadStepper>;

/** A lead somebody has reached out to: two rungs filled, three moves open. */
export const Contacted: Story = { render: ladder({}) };

/** The first rung, which is where most leads sit: nothing has happened yet. */
export const New: Story = { render: ladder({ lead: { status: "new" } }) };

/** The system moved this one on a booked meeting. Who moved it reads in
 *  the details' Status row, not under the ladder. */
export const MovedByTheSystem: Story = {
  render: ladder({
    lead: {
      status: "engaged",
      status_set_by: "system",
      qualification_evidence: {
        trigger: "meeting_booked",
        occurred_at: "2026-08-23T14:00:00Z",
      },
    },
  }),
};

/** Qualified: the ladder is finished, every step is a fact, and the one
 *  sentence on why the moves are refused is named by each, never repeated. */
export const Qualified: Story = {
  render: ladder({
    lead: { status: "promoted", archived_at: "2026-08-25T10:00:00Z" },
    closed: true,
  }),
};

/** Disqualified: the other closure, drawn the same way. Both terminal steps
 *  sit apart from the three open ones — they are where the ladder ends. */
export const Disqualified: Story = {
  render: ladder({
    lead: { status: "disqualified", archived_at: "2026-08-25T10:00:00Z" },
    closed: true,
  }),
};

/** A step already sent: every control is inert while the write is in flight,
 *  so a second press cannot race the first for the same If-Match. */
export const Saving: Story = { render: ladder({ pending: true }) };

/** At 390px the ladder wraps to as many rows as it needs — the steps keep
 *  their order and their separators rather than being crushed onto one line. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: ladder({ lead: { status: "engaged" } }),
};

/** The bars shape How it stands draws: the day the lead entered each rung,
 *  the current one marked as now, and the two ways out as chips. */
export const BarsWithDates: Story = {
  render: ladder({
    lead: { status: "engaged", status_set_by: "system" },
    bars: { new: "2 Aug", contacted: "4 Aug", engaged: "9 Aug · now" },
  }),
};

/** A rung with no recorded move draws no date rather than a guess. */
export const BarsUndated: Story = {
  render: ladder({ lead: { status: "contacted" }, bars: { new: "20 Aug" } }),
};
