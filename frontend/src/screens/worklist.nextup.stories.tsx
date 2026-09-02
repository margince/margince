// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { StoryProviders } from "./story-utils";
import { NextUp } from "./worklist.nextup";

// What follows the focus card — see worklist.nextup.tsx for why it is bounded,
// and for why these rows may all turn out to be one kind of work.

type WorklistItem = components["schemas"]["WorklistItem"];

const meta: Meta<typeof NextUp> = {
  title: "Records/Worklist/Next up",
  component: NextUp,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof NextUp>;

// The list as a reader usually meets it: three different kinds of work, each
// naming the record it is about.
export const ThreeDifferentJobs: Story = {
  args: {
    items: [
      {
        id: "01a05500-0000-7000-8000-000000000001",
        source: "customer_waiting",
        category: "customer_waiting",
        level: 1,
        consequence: "buyer_waits",
        title: "Re: pricing for the second site",
        because: [],
        actions: ["act"],
        primary_action: "act",
        subject: {
          type: "person",
          id: "01a05500-0000-7000-8000-000000000001",
          label: "Marta Kowalski",
        },
      },
      {
        id: "01a05500-0000-7000-8000-000000000002",
        source: "deal_at_risk",
        category: "deals_at_risk",
        level: 3,
        consequence: "deal_slips_past_close",
        title: "Acme Expansion",
        because: [],
        actions: ["open"],
        primary_action: "open",
        subject: {
          type: "deal",
          id: "01a05500-0000-7000-8000-000000000002",
          label: "Acme Expansion",
        },
      },
      {
        id: "01a05500-0000-7000-8000-000000000003",
        source: "task",
        category: "tasks",
        level: 4,
        consequence: "task_slips",
        title: "Send the signed order form",
        because: [],
        actions: ["complete"],
        primary_action: "complete",
        subject: {
          type: "organization",
          id: "01a05500-0000-7000-8000-000000000003",
          label: "Northwind Ltd",
        },
      },
    ] satisfies WorklistItem[],
  },
};

// A row filed under no record has nowhere for a link to GO, so its title draws
// as plain text rather than as a link to nothing. Kept as a story because no
// other surface renders that branch, and a dead link is the kind of thing only
// a rendered page shows.
export const ARowWithNowhereToGo: Story = {
  args: {
    items: [
      {
        id: "01a05500-0000-7000-8000-000000000004",
        source: "task",
        category: "tasks",
        level: 4,
        consequence: "task_slips",
        title: "Write up the quarterly territory plan",
        because: [],
        actions: ["complete"],
        primary_action: "complete",
      },
    ] satisfies WorklistItem[],
  },
};

// One kind of work, three times over. This is what the ranking's missing
// anti-monopoly cap looks like on the page: the component is behaving
// correctly, and the day still reads as a single sentence repeated.
export const AllOneKindOfWork: Story = {
  args: {
    items: [1, 2, 3].map((n) => ({
      id: `01a05500-0000-7000-8000-00000000001${n}`,
      source: "bounce" as const,
      category: "system" as const,
      level: 5,
      consequence: "customer_never_received" as const,
      title: `Message to contact ${n} bounced`,
      because: [],
      actions: ["act" as const],
      primary_action: "act" as const,
    })) satisfies WorklistItem[],
  },
};
