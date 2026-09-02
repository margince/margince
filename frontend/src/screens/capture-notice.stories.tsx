// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CaptureNotice } from "./capture-notice";
import { StoryProviders } from "./story-utils";

// What a person is told before their mailbox is read. One state, because it has
// one: it takes no props, reads nothing, and says the same thing on every
// connect surface — which is the point of it being a component rather than
// three copies of a paragraph.
//
// The story exists so the WORDS are reviewable on their own, away from the form
// that hosts them. This is the sentence the DACH compliance package is about,
// and it is easier to notice that a claim has stopped being true when it is not
// buried between a port field and a password.

const meta: Meta<typeof CaptureNotice> = {
  title: "Settings/You/Connections/Capture notice",
  component: CaptureNotice,
};
export default meta;
type Story = StoryObj<typeof CaptureNotice>;

export const Default: Story = {
  render: () => (
    <StoryProviders>
      <CaptureNotice />
    </StoryProviders>
  ),
};
