// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { PageAsideProvider, usePageAside } from "../app/pageaside";
import { ProjectTabs } from "./projecttabs";
import { StoryProviders } from "./story-utils";

// The strip on its own, away from the project page, because the thing to look
// at is the ROW rather than the page under it: one tab that is already the
// page the reader is on, and the details switch at the far end doing the work
// that earns the row.

const meta: Meta<typeof ProjectTabs> = {
  title: "Records/Project/Tabs",
  component: ProjectTabs,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProjectTabs>;

// The switch draws only for a screen that OFFERS a pane, so the story says it
// has one the way a screen does — through the hook — rather than reaching past
// the contract to set the flag itself.
function Row() {
  usePageAside();
  return <ProjectTabs />;
}

function row(open: boolean) {
  return () => (
    <StoryProviders>
      <PageAsideProvider open={open}>
        <Row />
      </PageAsideProvider>
    </StoryProviders>
  );
}

/** The pane standing open: the switch reads as pressed. */
export const DetailsOpen: Story = { render: row(true) };

/** Folded away: the same row, the switch unpressed. */
export const DetailsFolded: Story = { render: row(false) };
