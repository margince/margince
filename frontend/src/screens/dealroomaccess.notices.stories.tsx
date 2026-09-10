// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IssuedNotice } from "./dealroomaccess.notices";
import { StoryProviders } from "./story-utils";

// What became of the invitation a rep just issued, in the two readings of one
// press. Their own frames because the link itself is shown once and never
// stored in clear, so neither state survives a reload — and because the second
// one is not a failure: the link in the field beside it is valid either way,
// and `warn` here would send a rep looking for a fault that does not exist.

const meta: Meta<typeof IssuedNotice> = {
  title: "Records/Deal room/Access notices",
  component: IssuedNotice,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof IssuedNotice>;

/** It went out to the buyer. The rep can still copy it from the field below. */
export const Mailed: Story = {
  render: () => (
    <StoryProviders>
      <IssuedNotice queued email="ada@brandt.example" />
    </StoryProviders>
  ),
};

/** No relay, so the link is the rep's to send — a fact, not a fault. */
export const NotMailed: Story = {
  render: () => (
    <StoryProviders>
      <IssuedNotice queued={false} email="ada@brandt.example" />
    </StoryProviders>
  ),
};

/** Both in dark, where `success` and `info` have to stay tellable apart. */
export const BothDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <StoryProviders>
      <div className="form-stack">
        <IssuedNotice queued email="ada@brandt.example" />
        <IssuedNotice queued={false} email="ada@brandt.example" />
      </div>
    </StoryProviders>
  ),
};
