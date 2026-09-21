// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import { ForcedPasswordChangeScreen } from "./forcedpassword";
import { StoryProviders } from "./story-utils";

// Where an installation's first admin lands while their password is still the
// one a deployment file handed them.
//
// The surface is the sign-in surface — same ground, same wordmark — because
// the contact IS authenticated and sending them to a login form would offer
// them credentials that work and change nothing. So what a reviewer is looking
// at here is whether the boundary reads as an explanation rather than a
// refusal, and whether the settings page's own card sits on this ground
// without looking borrowed. It is the same card, reached a second way.

const meta: Meta<typeof ForcedPasswordChangeScreen> = {
  title: "Signed out/Forced password change",
  component: ForcedPasswordChangeScreen,
  // The surface owns the whole viewport (its ambient ground is the page), so
  // the catalog's 2rem frame would show it inset from a window it fills.
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: { onChanged: () => {} },
};
export default meta;

type Story = StoryObj<typeof ForcedPasswordChangeScreen>;

/** The boundary as the contact meets it: the reason, then the one way on. */
export const AtTheBoundary: Story = {};

/**
 * The same boundary dark. The ambient ground, the card and the fields are
 * three elevations a darker palette pushes together, and this screen stacks
 * all three.
 */
export const AtTheBoundaryDark: Story = { globals: { theme: "dark" } };

/**
 * The form the verb opens, which is where the contact actually spends their
 * time here — and the half of this screen that is not visible until something
 * is pressed.
 */
export const TheFormOpen: Story = {
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Change password" }),
    );
    await screen.findByRole("dialog");
  },
};
