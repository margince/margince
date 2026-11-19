// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import type { Tag } from "./tagadmin.queries";
import { TagDialog } from "./tagdialog";

// The dialog that coins a tag or corrects one, alone.

const VOCABULARY: readonly Tag[] = [
  { id: "t-1", name: "Key Account", color: "amber", version: 3 },
  { id: "t-2", name: "Churn Risk", color: "rose", version: 1 },
];

function Dialog({ existing }: Readonly<{ existing?: Tag }>) {
  installFetchStub({
    "POST /tags": () =>
      jsonResponse(
        { status: 409, code: "conflict", detail: 'tag "Churn Risk": conflict' },
        409,
      ),
  });
  return (
    <StoryProviders>
      <TagDialog
        existing={existing}
        vocabulary={VOCABULARY}
        onClose={() => undefined}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof TagDialog> = {
  title: "Settings/Sales/Records and fields/Tag dialog",
  component: TagDialog,
};
export default meta;
type Story = StoryObj<typeof TagDialog>;

/** A new word, before anything is typed. */
export const Add: Story = { render: () => <Dialog /> };

/** Correcting a word, with its description and the suggestion switch. */
export const Edit: Story = {
  render: () => <Dialog existing={VOCABULARY[1]} />,
};

/** A name another tag holds: the name field says so, once. */
export const NameTaken: Story = {
  render: () => <Dialog />,
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.type(
      await page.findByRole("textbox", { name: "Name" }),
      "Churn Risk",
    );
    await userEvent.click(page.getByRole("button", { name: "Add" }));
    await expect(
      await page.findByText("A tag with this name already exists."),
    ).toBeVisible();
  },
};

/** Add pressed on an empty name: the field says what it needs. */
export const NameRequired: Story = {
  render: () => <Dialog />,
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(await page.findByRole("button", { name: "Add" }));
    await expect(await page.findByText("Name is required.")).toBeVisible();
  },
};
