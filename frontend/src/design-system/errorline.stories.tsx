// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { ProblemError } from "../screens/common";
import { Button, TextInput } from "./atoms";
import { ErrorLine } from "./errorline";
import { Row, Stack } from "./stack";

// The one refusal line, in each of the shapes a caller hands it. Flip the
// Theme control: the ink is `--dangerText`, which is lifted in dark.
const meta: Meta = {
  title: "Design System/ErrorLine",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;

const conflict = new ProblemError({
  code: "version_conflict",
  detail: "Somebody else changed this record. Re-read it and try again.",
});

export const MutationFailure: Story = {
  name: "A mutation's error",
  render: () => <ErrorLine error={conflict} />,
};

export const Sentence: Story = {
  name: "A sentence the caller translated",
  render: () => (
    <ErrorLine>This message has more than one recipient.</ErrorLine>
  ),
};

export const WithActions: Story = {
  name: "With a verb on the line",
  render: () => (
    <ErrorLine
      error={conflict}
      actions={
        <Button variant="ghost" onClick={() => undefined}>
          Re-read
        </Button>
      }
    />
  ),
};

export const DescribesAControl: Story = {
  name: "Describing the control above it",
  render: () => (
    <Stack gap="2">
      <TextInput
        aria-label="VAT number"
        aria-describedby="vat-refusal"
        aria-invalid
        defaultValue="DE12"
      />
      <ErrorLine id="vat-refusal">
        A VAT number has nine digits after the country code.
      </ErrorLine>
    </Stack>
  ),
};

export const Inline: Story = {
  name: "Inline, in its control's row",
  render: () => (
    <Row gap="2">
      <TextInput
        aria-label="Discount"
        aria-describedby="discount-refusal"
        aria-invalid
        defaultValue="140"
      />
      <ErrorLine inline id="discount-refusal">
        A discount cannot exceed 100%.
      </ErrorLine>
    </Row>
  ),
};

export const Standing: Story = {
  name: "Standing: true when the surface drew, not announced",
  render: () => (
    <ErrorLine standing>
      You can read this room, but only its owner can post in it.
    </ErrorLine>
  ),
};

export const NothingToReport: Story = {
  name: "No error: draws nothing",
  render: () => <ErrorLine error={null} />,
};
