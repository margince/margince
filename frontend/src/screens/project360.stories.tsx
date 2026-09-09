// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { PageAsideProvider } from "../app/pageaside";
import { ProjectScreen } from "./project360";
import { project360 } from "./projects.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The project page, in the two states its own chrome can be in: the details
// pane standing beside the work, and folded away with the work at full width.
// Both are drawn from `projects.fixtures.ts`, the 360 the project suites build
// from — a story that hand-rolled a second one would be a second answer to
// what a project looks like, and the two would drift.
//
// What to read here is the ROW under the header: the tab strip carries the
// details switch at its end, which is where every other record page keeps it.

const meta: Meta<typeof ProjectScreen> = {
  title: "Records/Project/Screen",
  component: ProjectScreen,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof ProjectScreen>;

// The pane's state is stated per story rather than inherited: `StoryProviders`
// mounts the shell's provider standing open, and the folded story is about the
// other setting, so it puts its own memory closer to the screen.
function page(open: boolean) {
  return () => {
    installFetchStub({
      // The grants the page's verbs ask before they draw — a rep working
      // their own projects, which is the seat the fixture's `writable: true`
      // describes.
      "GET /me": meRoute({
        project: ["read", "create", "update", "delete"],
        deal: ["read", "create"],
      }),
      "GET /projects/pr-1/360": () => jsonResponse(project360()),
    });
    return (
      <StoryProviders>
        <PageAsideProvider open={open}>
          <ProjectScreen id="pr-1" />
        </PageAsideProvider>
      </StoryProviders>
    );
  };
}

/** The pane open beside the work, under the tab row that carries its switch. */
export const DetailsOpen: Story = { render: page(true) };

/** Folded away: the work takes the whole width and the switch reads "show". */
export const DetailsFolded: Story = { render: page(false) };
