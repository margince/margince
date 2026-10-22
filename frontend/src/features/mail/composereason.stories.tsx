// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { ContactReasonForm } from "./composereason";

const meta: Meta<typeof ContactReasonForm> = {
  title: "Patterns/Compose mail/Contact reason",
  component: ContactReasonForm,
};
export default meta;

/** A reason to write, recorded under the rep's name where a send is refused
 *  for lack of one. Nothing typed yet, so the verb waits. */
export const Empty: StoryObj = {
  render: () => (
    <StoryProviders>
      <ContactReasonForm
        target={{
          contactId: "c-1",
          address: "anna@example.test",
          category: "requested_followup",
        }}
        onRecorded={() => {}}
      />
    </StoryProviders>
  ),
  play: async () => {
    await expect(
      await screen.findByLabelText("Why you may write to anna@example.test"),
    ).toBeInTheDocument();
  },
};
