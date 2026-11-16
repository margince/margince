// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fireEvent, userEvent, within } from "storybook/test";
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
        <>
          <p>
            Emails of this kind are no longer sent to you. Security and service
            messages you need for something you requested are not affected.
          </p>
          <p>
            Your other choices stay as they are, and you can change them at any
            time from the link at the foot of any message.
          </p>
          <p>
            No sign-in is needed. This personal link controls only your email
            preferences, so do not share it.
          </p>
        </>
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

/** The column's painted top after the observer has answered the last change. */
function paintedTop(box: Element): Promise<number> {
  return new Promise((resolve) => {
    requestAnimationFrame(() =>
      requestAnimationFrame(() => resolve(box.getBoundingClientRect().top)),
    );
  });
}

/** Resolves as the glide starts, before it has travelled. */
function glideStarts(column: Element): Promise<void> {
  return new Promise((resolve) => {
    const watch = new MutationObserver(() => {
      if (column.hasAttribute("data-gliding")) {
        watch.disconnect();
        resolve();
      }
    });
    watch.observe(column, { attributes: true });
  });
}

/** A second press mid-glide carries on from where the column is painted. */
export const PressedAgainMidGlide: Story = {
  ...GrowsOnAPress,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const toggle = await canvas.findByRole("button", {
      name: "Show the details",
    });
    const column = toggle.closest(".pref-center");
    await expect(column).not.toBeNull();
    if (column === null) return;
    const started = glideStarts(column);
    await userEvent.click(toggle);
    await started;
    const before = toggle.getBoundingClientRect().top;
    fireEvent.click(toggle);
    const after = await paintedTop(toggle);
    await expect(Math.abs(after - before)).toBeLessThan(8);
  },
};
