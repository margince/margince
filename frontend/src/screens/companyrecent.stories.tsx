// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { CompanyRecentList } from "./companyrecent";
import { StoryProviders } from "./story-utils";

// "What happened lately" (co360's own row-per-exchange reading): the deal a
// row is ABOUT, named when `nameOf` can resolve it and left the honest
// unnamed phrase when it cannot. The neutral kind chip never borrows the
// AI-provenance indigo for a human's own correspondence.

type Activity = components["schemas"]["Activity"];

const base: Activity = {
  id: "a-1",
  kind: "email",
  subject: "Renewal terms",
  occurred_at: "2026-08-20T09:00:00Z",
  is_done: false,
  direction: "inbound",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-20T09:00:00Z",
};

function List({ activities }: Readonly<{ activities: Activity[] }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 640 }}>
        <CompanyRecentList
          activities={activities}
          nameOf={(entityType, entityId) =>
            entityType === "deal" && entityId === "d-1"
              ? "Acme Expansion"
              : undefined
          }
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof List> = {
  title: "Records/Company 360/What happened lately",
  component: List,
};
export default meta;
type Story = StoryObj<typeof List>;

// A row about a deal `nameOf` can resolve: "on Acme Expansion" is a fact a
// rep acts on, not a click to find out.
export const DealResolved: Story = {
  render: () => (
    <List
      activities={[
        {
          ...base,
          links: [{ entity_type: "deal", entity_id: "d-1" }],
        },
      ]}
    />
  ),
};

// The same shape of row, but the reading this list was built from no longer
// carries that deal (a closed deal, a capped list). The fallback phrase, not
// a blank line where a name would have gone.
export const DealNameMissing: Story = {
  render: () => (
    <List
      activities={[
        {
          ...base,
          id: "a-2",
          links: [{ entity_type: "deal", entity_id: "d-missing" }],
        },
      ]}
    />
  ),
};

// Every kind the row draws, each with its own neutral chip and its own
// direction phrase: an email they wrote, a call we made, a task with no
// direction at all, and a note that states none either.
export const MixedKinds: Story = {
  render: () => (
    <List
      activities={[
        {
          ...base,
          id: "a-3",
          kind: "email",
          subject: "Re: Renewal terms",
          direction: "inbound",
          occurred_at: "2026-08-20T09:00:00Z",
        },
        {
          ...base,
          id: "a-4",
          kind: "call",
          subject: undefined,
          direction: "outbound",
          duration_seconds: 900,
          occurred_at: "2026-08-19T14:00:00Z",
        },
        {
          ...base,
          id: "a-5",
          kind: "task",
          subject: "Send updated MSA",
          direction: undefined,
          occurred_at: "2026-08-18T11:00:00Z",
        },
        {
          ...base,
          id: "a-6",
          kind: "note",
          subject: "Prefers async updates",
          direction: undefined,
          occurred_at: "2026-08-17T16:00:00Z",
        },
      ]}
    />
  ),
};

// The same four kinds in dark: each chip is neutral by rule, never the
// AI-provenance indigo, so this is the story that shows whether that
// neutrality still holds once the panel itself is dark.
export const MixedKindsDark: Story = {
  ...MixedKinds,
  globals: { theme: "dark" },
};
