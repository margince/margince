// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { ContactReasonPick } from "./composereason";

const meta: Meta<typeof ContactReasonPick> = {
  title: "Patterns/Compose mail/Contact reason",
  component: ContactReasonPick,
};
export default meta;

/** A reason to write, picked where a send is refused for lack of one. The
 *  pick records it under the rep's name. Nothing picked yet. */
export const Empty: StoryObj = {
  render: () => (
    <StoryProviders>
      <ContactReasonPick
        target={{
          contactId: "c-1",
          address: "anna@example.test",
          category: "requested_followup",
        }}
      />
    </StoryProviders>
  ),
  play: async () => {
    await expect(
      await screen.findByRole("combobox", {
        name: "Why you may write to anna@example.test",
      }),
    ).toBeInTheDocument();
  },
};
