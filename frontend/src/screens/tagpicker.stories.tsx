// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { screen, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { AddTagDialog } from "./tagpicker";

// Pick a word the workspace already has. The dialog cannot coin one, so what
// is worth a picture is the pair that stops a reader coining a near-duplicate
// anyway: the whole catalog, and a catalog that was CUT and says so. The
// no-match plate is reached by typing and belongs to the unit test beside this.

const ORG = "01a06151-0000-7000-8000-000000000001";

const VOCABULARY = [
  { id: "t-1", workspace_id: "w", name: "Key Account", color: "sky" },
  { id: "t-2", workspace_id: "w", name: "Renewal", color: "lime" },
];

// This surface is a Modal, portalled to document.body, so `#storybook-root`
// holds the preview decorator and nothing else however well the dialog renders
// — and `layout: "fullscreen"` removes even that, which leaves the render gate
// watching an empty root and reporting a dialog that mounted perfectly as one
// that never rendered.
//
// The root filling is then only evidence that the DECORATOR ran, so the frames
// here drive a play that names what they expect. A rejecting play IS a failure
// the gate reports, which is what makes their green worth something.
const meta: Meta<typeof AddTagDialog> = {
  title: "Patterns/Add tag",
  component: AddTagDialog,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText("Key Account");
  },
};
export default meta;
type Story = StoryObj<typeof AddTagDialog>;

function Served({
  truncated,
  children,
}: Readonly<{ truncated: boolean; children: ReactNode }>) {
  installFetchStub({
    "GET /me": meRoute({ tag: ["read"] }),
    "GET /tags": () =>
      jsonResponse({
        data: VOCABULARY,
        page: { has_more: truncated, next_cursor: null },
      }),
  });
  return <StoryProviders>{children}</StoryProviders>;
}

/** The whole vocabulary fits, so the dialog says nothing about its own length. */
export const WholeCatalog: Story = {
  render: () => (
    <Served truncated={false}>
      <AddTagDialog
        entityType="organization"
        entityID={ORG}
        current={[]}
        onClose={() => undefined}
      />
    </Served>
  ),
};

/**
 * The catalog was cut. Without the notice a word past the cap reads as a word
 * the workspace does not have, and the reader asks for a duplicate.
 */
export const CatalogCut: Story = {
  render: () => (
    <Served truncated>
      <AddTagDialog
        entityType="organization"
        entityID={ORG}
        current={[]}
        onClose={() => undefined}
      />
    </Served>
  ),
};
