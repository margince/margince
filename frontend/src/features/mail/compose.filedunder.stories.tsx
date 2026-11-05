// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
import type { Meta, StoryObj } from "@storybook/react-vite";
import { DockedWindow } from "../../design-system/dockedwindow";
import { StoryProviders } from "../../testing/story-utils";
import { ComposeAbout } from "./compose.parts";

// The line names the record a draft files under, for a reader who opened the
// composer from somewhere that is not that record. Drawn in the window's dark
// bar, the only place it is ever shown, so the story shows its real contrast.
const meta = {
  title: "Patterns/Compose mail/Filed under",
  component: ComposeAbout,
  parameters: { layout: "padded" },
  args: { kind: "contact", id: "c-1", name: "Anna Weber" },
  render: (args) => (
    <DockedWindow
      title="Re: Budget questions"
      aside={<ComposeAbout {...args} />}
      state="open"
      onStateChange={() => undefined}
      onClose={() => undefined}
    >
      <div />
    </DockedWindow>
  ),
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof ComposeAbout>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Contact: Story = {};

export const Company: Story = {
  args: { kind: "company", id: "co-1", name: "Newsky Logistics" },
};

// A name long enough to meet the window's edge, since the composer is as narrow
// as a reader drags it.
export const LongName: Story = {
  args: { name: "Internationale Speditions- und Logistikgesellschaft" },
};

// No record means no line: a reader is not offered a way to nothing.
export const NoRecord: Story = { args: { id: undefined } };

export const ContactDark: Story = { globals: { theme: "dark" } };
