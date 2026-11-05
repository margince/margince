// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { LinkDialog } from "./linkdialog";

// The editor owns the selection; the dialog owns only the address.
const meta: Meta<typeof LinkDialog> = {
  title: "Components/Overlays and layering/Link dialog",
  component: LinkDialog,
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
    href: "",
    onApply: () => undefined,
    onClose: () => undefined,
  },
};
export default meta;
type Story = StoryObj<typeof LinkDialog>;

export const NewLink: Story = {
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await expect(dialog.getByLabelText("Web or email address")).toHaveValue("");
    await expect(dialog.queryByRole("button", { name: "Remove link" })).toBe(
      null,
    );
  },
};

// The caret sits in a link, so its address is the start and it can come off.
export const EditingLink: Story = {
  args: {
    href: "https://example.com/pricing",
    onRemove: () => undefined,
  },
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await expect(dialog.getByLabelText("Web or email address")).toHaveValue(
      "https://example.com/pricing",
    );
    // Not toBeVisible: the dialog is still fading in when play runs.
    await expect(
      dialog.getByRole("button", { name: "Remove link" }),
    ).toBeEnabled();
  },
};

// A script address is refused on the field, in words.
export const RefusedAddress: Story = {
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    const field = dialog.getByLabelText("Web or email address");
    await userEvent.type(field, "javascript:alert(1)");
    await userEvent.click(dialog.getByRole("button", { name: "Apply" }));
    await expect(field).toHaveAttribute("aria-invalid", "true");
    await expect(
      dialog.getByText(/This is not a web or email address/),
    ).toBeInTheDocument();
  },
};

export const RefusedAddressDark: Story = {
  ...RefusedAddress,
  globals: { theme: "dark" },
};

export const PhoneRefusedAddress: Story = {
  ...RefusedAddress,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
