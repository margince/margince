// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { Button, OverflowMenu, SegmentedControl } from "../design-system/atoms";
import { FocusedHead, FocusedPending, FocusedState } from "./filterhead";
import { StoryProviders } from "./story-utils";

// The head of a focused Filters and views page: its one h1, with the control
// the page reads from or the verbs that act on what it holds, and the same
// heading while the page waits for what it will name.
const meta: Meta<typeof FocusedHead> = {
  title: "Patterns/Filters and views/Focused head",
  component: FocusedHead,
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

type Story = StoryObj<typeof FocusedHead>;

/** The page column a head sits at the top of. */
function Page({ children }: Readonly<{ children: ReactNode }>) {
  return <div className="wrap filters-screen">{children}</div>;
}

// A new filter: the record type beside the name.
export const WithControl: Story = {
  render: () => (
    <Page>
      <FocusedHead
        title="New contact filter"
        control={
          <SegmentedControl
            options={["contacts", "companies", "deals", "leads"] as const}
            value="contacts"
            onChange={() => {}}
            labels={{
              contacts: "Contacts",
              companies: "Companies",
              deals: "Deals",
              leads: "Leads",
            }}
            label="Record type"
          />
        }
      />
    </Page>
  ),
};

// An opened item: what it is under its name, and its verbs behind More.
export const WithFactsAndMore: Story = {
  render: () => (
    <Page>
      <FocusedHead
        title="Berlin contacts"
        facts="Saved view · Contacts · Only me"
        actions={
          <OverflowMenu label="More for Berlin contacts">
            <Button>Rename</Button>
            <Button variant="danger">Delete view</Button>
          </OverflowMenu>
        }
      />
    </Page>
  ),
};

// Still reading what the page will name: the fallback name heads it.
export const Pending: Story = {
  render: () => <FocusedPending title="Saved view" label="Loading…" />,
};

// Nothing the page can open: why, under the name it would have worn, and the
// way back to the library.
export const NothingToOpen: Story = {
  render: () => (
    <FocusedState
      title="Saved view"
      sentence="This saved view was deleted or cannot be found."
    />
  ),
};
