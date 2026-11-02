// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { LockedValue, RecordRefusalContext } from "./lockedvalue";

const meta: Meta = {
  title: "Components/Forms and input/Locked value",
  component: LockedValue,
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ width: 280 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;

// A field locked for a reason of its own: the lock trails the value and a
// press opens the reason.
export const FieldLock: Story = {
  render: () => (
    <LockedValue label="Domain" reason="Domains come from the company's sites.">
      acme.example
    </LockedValue>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Why Domain is locked" }),
    );
    await expect(
      await within(document.body).findByText(
        "Domains come from the company's sites.",
      ),
    ).toBeVisible();
  },
};

// The whole record is locked and its banner says why, so the value is plain.
export const RecordLock: Story = {
  render: () => (
    <RecordRefusalContext.Provider value="This company is archived.">
      <LockedValue label="Domain" reason="This company is archived.">
        acme.example
      </LockedValue>
    </RecordRefusalContext.Provider>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByRole("button")).toBeNull();
  },
};
