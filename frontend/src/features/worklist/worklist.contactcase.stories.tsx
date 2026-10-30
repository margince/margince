// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { Panel, PanelRow } from "../../design-system/panel";
import {
  ANA,
  BEN,
  CLEO,
  contactCase,
} from "../../testing/contactcase.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ContactCaseDecision } from "./worklist.contactcase";

// A group of contacts that may be duplicates, on its Worklist row. Framed in a
// Panel row, which is where the Worklist draws it.

type WorklistItem = components["schemas"]["WorklistItem"];
type AttentionContactCase = components["schemas"]["AttentionContactCase"];

const meta: Meta<typeof ContactCaseDecision> = {
  title: "Records/Worklist/Contact case",
  component: ContactCaseDecision,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      stubWithSession(
        {
          [`GET /dedupe/contact-cases/${ANA}`]: () =>
            jsonResponse(contactCase()),
        },
        {},
      );
      return (
        <StoryProviders>
          <Panel>
            <PanelRow>
              <Story />
            </PanelRow>
          </Panel>
        </StoryProviders>
      );
    },
  ],
};
export default meta;

type Story = StoryObj<typeof ContactCaseDecision>;

function caseRow(over: Partial<AttentionContactCase> = {}): WorklistItem {
  return {
    occurred_precision: "instant",
    id: ANA,
    source: "contact_case",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    because: [],
    actions: ["merge"],
    contact_case: {
      anchor_id: ANA,
      revision: "rev-1",
      members: [
        { id: ANA, label: "Mira Kovac", detail: "mira@northwind.de" },
        { id: BEN, label: "Mira Kovač", detail: "m.kovac@northwind.de" },
        { id: CLEO, label: "M. Kovac", detail: "mira.k@gmail.com" },
      ],
      owners: ["Ana Owner", "Ben Seller", "Cleo Reader"],
      can_merge: true,
      merge_mode: "ordinary",
      ...over,
    },
  };
}

/** The reader may change every member. */
export const Ordinary: Story = { args: { item: caseRow() } };

/** The setting lets any owner settle it, and the row says so. */
export const Delegated: Story = {
  args: { item: caseRow({ merge_mode: "delegated" }) },
};

/** The reader cannot merge; the review says who can. */
export const NotAllowed: Story = {
  args: { item: caseRow({ can_merge: false, merge_mode: "none" }) },
};

/** Too many members for one review. */
export const TooLarge: Story = {
  args: {
    item: caseRow({ can_merge: false, merge_mode: "none", too_large: true }),
  },
};
