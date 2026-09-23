// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { ProblemError } from "../screens/common";
import { InlineText } from "./inlinetext";

const meta: Meta = {
  title: "Design System/Inline text editing",
  decorators: [
    (Story) => (
      <LocaleProvider>
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;
function Example({ multiline = false }: { multiline?: boolean }) {
  const [value, setValue] = useState(
    multiline ? "A paragraph with intentional whitespace." : "14.60",
  );
  return (
    <InlineText
      label={multiline ? "Brief" : "Value"}
      placeholder="Not set"
      value={value}
      multiline={multiline}
      type={multiline ? "text" : "number"}
      step="any"
      canEdit
      onSave={async (next) => setValue(next)}
    />
  );
}
export const FractionalNumber: Story = { render: () => <Example /> };
export const Paragraph: Story = { render: () => <Example multiline /> };

// A refused save keeps the draft open and says why on the ErrorLine beneath it.
export const RefusedSave: Story = {
  render: () => (
    <InlineText
      label="Website"
      placeholder="Not set"
      value="acme.example"
      canEdit
      onSave={async () => {
        throw new ProblemError({
          code: "version_conflict",
          detail: "Somebody else changed this record.",
        });
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("button"));
    const field = await canvas.findByRole("textbox");
    await userEvent.type(field, "s{Enter}");
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      "Somebody else changed this record.",
    );
  },
};
