// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
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

// Nothing typed yet: the save says the name is required and goes to it.
export const Create: Story = {
  args: { onSave: fn() },
  play: async ({ args }) => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await userEvent.click(dialog.getByRole("button", { name: "Add reason" }));
    await waitFor(() =>
      expect(dialog.getByText("Reason is required.")).toBeVisible(),
    );
    await expect(dialog.getByRole("textbox", { name: /Reason/ })).toHaveFocus();
    await expect(args.onSave).not.toHaveBeenCalled();
  },
};

// A rename starts from the current name, and the unchanged name saves nothing.
export const Rename: Story = {
  args: {
    title: "Rename reason",
    initial: "Bad timing",
    confirmLabel: "Save name",
    onSave: fn(),
    onClose: fn(),
  },
  play: async ({ args }) => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await expect(dialog.getByRole("textbox", { name: /Reason/ })).toHaveValue(
      "Bad timing",
    );
    await userEvent.click(dialog.getByRole("button", { name: "Save name" }));
    await expect(args.onSave).not.toHaveBeenCalled();
    await expect(args.onClose).toHaveBeenCalled();
  },
};

// A label that repeats the title is not drawn twice; the field keeps its name.
export const RenameTitleIsLabel: Story = {
  args: {
    title: "Deal name",
    label: "Deal name",
    initial: "Berlin rollout",
    confirmLabel: "Save",
  },
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    const printed = dialog
      .getAllByText("Deal name")
      .filter((node) => !node.classList.contains("sr-only"));
    await expect(printed).toHaveLength(1);
    await expect(
      dialog.getByRole("textbox", { name: "Deal name" }),
    ).toHaveValue("Berlin rollout");
  },
};

// A rename shows a plain line under the field, never the list's internal key.
export const RenameWithHint: Story = {
  args: {
    title: "Rename field",
    label: "Field name",
    initial: "Fleet size",
    confirmLabel: "Save",
    hint: "Existing records keep the old name until you save.",
  },
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    const field = dialog.getByLabelText("Field name");
    const described = document.getElementById(
      field.getAttribute("aria-describedby") ?? "",
    );
    await expect(described).toHaveTextContent(
      "Existing records keep the old name until you save.",
    );
    await expect(described?.querySelector("code")).toBeNull();
  },
};

export const RenameWithHintPhone: Story = {
  ...RenameWithHint,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
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

// The server refused the name as too long. The box takes no more than its bound.
export const NameTooLong: Story = {
  args: {
    title: "Rename reason",
    initial: "Went quiet",
    confirmLabel: "Save name",
    maxLength: 120,
    nameProblem: "Reason: at most 120 characters.",
  },
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    const field = dialog.getByLabelText("Reason");
    await expect(field).toHaveAttribute("data-character-limit", "120");
    await userEvent.type(field, " after the demo{Enter}");
    await expect(field).toHaveAccessibleDescription(
      "Reason: at most 120 characters.",
    );
  },
};

export const DuplicateNamePhone: Story = {
  ...DuplicateName,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
