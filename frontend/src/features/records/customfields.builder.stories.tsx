// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { FieldBuilder } from "./customfields.builder";

const meta: Meta<typeof FieldBuilder> = {
  title: "Settings/Sales/Records and fields/Field builder",
  component: FieldBuilder,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof FieldBuilder>;

const noop = () => {};

// At 390px the type is one field-width dropdown, and the field it reveals sits
// under it.
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => (
    <StoryProviders>
      <FieldBuilder
        object="deal"
        pending={false}
        onSubmit={noop}
        onCancel={noop}
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(canvas.getByLabelText("Label"), "Ceiling");
    await userEvent.click(canvas.getByRole("combobox", { name: "Type" }));
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await page.findByRole("option", { name: "Currency" }),
    );
    await canvas.findByLabelText("Currency code");
  },
};
