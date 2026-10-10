// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { NameDialog } from "./namedialog";

// The caller owns the write, so pending and both refusals arrive as props.
const meta: Meta<typeof NameDialog> = {
  title: "Components/Overlays and layering/Name dialog",
  component: NameDialog,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
  args: {
    open: true,
    onClose: () => undefined,
    onSave: () => undefined,
    title: "New reason",
    label: "Reason",
    confirmLabel: "Add reason",
  },
};
export default meta;
type Story = StoryObj<typeof NameDialog>;

// Nothing typed yet: the save is refused as a state, not as a write in flight.
export const Create: Story = {
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await expect(
      dialog.getByRole("button", { name: "Add reason" }),
    ).toBeDisabled();
  },
};

// A rename starts from the current name, and the unchanged name saves nothing.
export const Rename: Story = {
  args: {
    title: "Rename reason",
    initial: "Bad timing",
    confirmLabel: "Save name",
  },
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await expect(dialog.getByLabelText("Reason")).toHaveValue("Bad timing");
    await expect(
      dialog.getByRole("button", { name: "Save name" }),
    ).toBeDisabled();
  },
};

export const Saving: Story = {
  args: { initial: "Went quiet", title: "Rename reason", pending: true },
};

// The write was refused for a reason that is not the name: the line under the form.
export const Refused: Story = {
  args: { problem: "Only an administrator can change this list." },
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await userEvent.type(dialog.getByLabelText("Reason"), "Went quiet");
    await dialog.findByRole("alert");
  },
};

// The name itself is taken: the field says so until the name changes.
export const DuplicateName: Story = {
  args: { nameProblem: "A reason with this name already exists." },
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await userEvent.type(dialog.getByLabelText("Reason"), "Bad timing{Enter}");
    await expect(dialog.getByLabelText("Reason")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  },
};

export const DuplicateNamePhone: Story = {
  ...DuplicateName,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
