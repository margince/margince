// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { LeadStepper } from "./leads.stepper";
import { installFetchStub, StoryProviders } from "./story-utils";

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
  readOnlyReason?: string;
}) {
  return () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <LeadStepper
          lead={lead(props.lead)}
          pending={props.pending ?? false}
          readOnlyReason={props.readOnlyReason}
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

/** The first rung, which is where most leads sit: nothing has happened yet
 *  and the line under the ladder says so rather than leaving it blank. */
export const New: Story = { render: ladder({ lead: { status: "new" } }) };

/** The system moved this one, reading a captured reply — the line names what
 *  it read and when, so a rep is not left asking who decided. */
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

/** Qualified: the ladder is finished, every step is a fact, and the reason
 *  the moves are refused is stated on each one rather than once elsewhere. */
export const Qualified: Story = {
  render: ladder({
    lead: { status: "promoted", archived_at: "2026-08-25T10:00:00Z" },
    readOnlyReason: "This lead is closed. It takes no further changes.",
  }),
};

/** Disqualified: the other closure, drawn the same way. Both terminal steps
 *  sit apart from the three open ones — they are where the ladder ends. */
export const Disqualified: Story = {
  render: ladder({
    lead: { status: "disqualified", archived_at: "2026-08-25T10:00:00Z" },
    readOnlyReason: "This lead is closed. It takes no further changes.",
  }),
};

/** A mirrored lead: the incumbent owns the lifecycle, so every step reads and
 *  none of them writes. The refusal is a different sentence from a closure's,
 *  because it is a different fact. */
export const MirrorRefusesTheWrite: Story = {
  render: ladder({
    readOnlyReason: "The connected system owns this lead's stage.",
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
