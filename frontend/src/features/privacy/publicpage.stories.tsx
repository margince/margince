// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Button } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { StoryProviders } from "../../testing/story-utils";
import { PublicPage } from "./publicpage";

// The frame every page a recipient opens from a message sits in. The screens'
// own stories show it full; this one shows the column gliding when a press
// grows it, which a resting picture cannot.

function GrowsOnPress() {
  const [open, setOpen] = useState(false);
  return (
    <PublicPage>
      <Heading size="xlarge" className="t-display">
        Stop receiving these emails?
      </Heading>
      <Button onClick={() => setOpen((was) => !was)}>
        {open ? "Hide the details" : "Show the details"}
      </Button>
      {open && (
        <p>
          Emails of this kind are no longer sent to you. Security and service
          messages you need for something you requested are not affected. Your
          other choices stay as they are, and you can change them at any time.
        </p>
      )}
    </PublicPage>
  );
}

const meta: Meta<typeof PublicPage> = {
  title: "Signed out/Public page frame",
  component: PublicPage,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof PublicPage>;

/** A short page, centred; a press that grows it glides the column up. */
export const GrowsOnAPress: Story = {
  render: () => (
    <StoryProviders>
      <GrowsOnPress />
    </StoryProviders>
  ),
};

export const GrowsOnAPressPhone: Story = {
  ...GrowsOnAPress,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
