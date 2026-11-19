// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryProviders } from "../testing/story-utils";
import { Button } from "./atoms";
import { OptionCount } from "./optioncount";

// The count beside an option's name, drawn inside the option's button as its
// hosts draw it, so the figure joins the button's name.
const meta: Meta<typeof OptionCount> = {
  title: "Components/Text and data display/Option count",
  component: OptionCount,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof OptionCount>;

function Host({
  name,
  ...count
}: Readonly<{ name: string } & Parameters<typeof OptionCount>[0]>) {
  return (
    <StoryProviders>
      <Button variant="ghost">
        {name}
        <OptionCount {...count} />
      </Button>
    </StoryProviders>
  );
}

/** A bare figure, where the option's name says what it counts. */
export const Plain: Story = {
  render: () => <Host name="Contacts" count={1240} />,
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", {
        name: /^Contacts\s*, 1,240$/,
      }),
    ).toBeVisible();
  },
};

/** A figure the name does not explain carries a label, read in its place. */
export const Labelled: Story = {
  render: () => (
    <Host name="Contacts" count={2} label="2 columns not imported" />
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", {
        name: /^Contacts\s*, 2 columns not imported$/,
      }),
    ).toBeVisible();
  },
};

export const LabelledDark: Story = {
  globals: { theme: "dark" },
  render: Labelled.render,
};
