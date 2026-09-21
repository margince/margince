// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { WriteRefused } from "./common";
import { ReadWarnings, SavedNotice } from "./company-context.notices";
import { StoryProviders } from "./story-utils";

// The three bands the company-context card says about itself, side by side,
// which is the one view the card itself never gives: the shared refusal, the
// confirmation a save leaves behind, and the caveats a website read returned.
// Read them in both themes — tone reaches the heading's ink and nothing else.

const meta: Meta = {
  title: "Records/Company 360/Context notices",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function Frame() {
  return (
    <StoryProviders>
      <div className="form-stack">
        <WriteRefused
          titleKey="settings.companySaveFailed"
          message="The website could not be reached."
        />
        <SavedNotice />
        <ReadWarnings
          warnings={[
            "brandt.example/imprint answered 403.",
            "brandt.example/about redirected off the domain.",
          ]}
        />
      </div>
    </StoryProviders>
  );
}

export const AllThree: Story = { render: () => <Frame /> };

export const AllThreeDark: Story = {
  globals: { theme: "dark" },
  render: () => <Frame />,
};

/** Nothing to say, so nothing is drawn — the caller states no condition. */
export const NoWarnings: Story = {
  render: () => (
    <StoryProviders>
      <ReadWarnings warnings={[]} />
    </StoryProviders>
  ),
};
