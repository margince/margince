// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { screen, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { AddTagPicker } from "./tagpicker";

// Pick a word the workspace already has. The picker cannot coin one, so what
// is worth a picture is the pair that stops a reader coining a near-duplicate
// anyway: the whole catalog, and a catalog that was CUT and says so. The
// no-match plate is reached by typing and belongs to the unit test beside this.

const COMPANY = "01a06151-0000-7000-8000-000000000001";

const VOCABULARY = [
  { id: "t-1", workspace_id: "w", name: "Key Account", color: "sky" },
  { id: "t-2", workspace_id: "w", name: "Renewal", color: "lime" },
];

// The panel is portalled to document.body, so the play finds it there, after
// pressing the trigger the story renders.
const meta: Meta<typeof AddTagPicker> = {
  title: "Patterns/Add tag",
  component: AddTagPicker,
  parameters: { layout: "padded" },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: "Add tag" }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText("Key Account");
  },
};
export default meta;
type Story = StoryObj<typeof AddTagPicker>;

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

/** The whole vocabulary fits, so the list says nothing about its own length. */
export const WholeCatalog: Story = {
  render: () => (
    <Served truncated={false}>
      <AddTagPicker entityType="company" entityID={COMPANY} current={[]} />
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
      <AddTagPicker entityType="company" entityID={COMPANY} current={[]} />
    </Served>
  ),
};

/** On a phone the same search and list fill a sheet. */
export const PhoneSheet: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => (
    <Served truncated={false}>
      <AddTagPicker entityType="company" entityID={COMPANY} current={[]} />
    </Served>
  ),
};
