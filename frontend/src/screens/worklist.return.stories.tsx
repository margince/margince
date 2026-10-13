// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { expect, within } from "storybook/test";
import { en } from "../i18n/en";
import { announceAddressChanged } from "../lib/router";
import { StoryProviders } from "./story-utils";
import { WorklistReturnLink } from "./worklist.return";

// A record opened from the expanded Worklist drawer, as its address carries it.
const FROM_THE_DRAWER =
  "#/contacts/c-1?filter=tasks&from=worklist&queue_scope=team&wide=1";

function OnTheRecord() {
  useEffect(() => {
    const before = window.location.hash;
    window.location.hash = FROM_THE_DRAWER;
    announceAddressChanged();
    return () => {
      window.location.hash = before;
      announceAddressChanged();
    };
  }, []);
  return (
    <StoryProviders>
      <WorklistReturnLink />
    </StoryProviders>
  );
}

const meta: Meta<typeof WorklistReturnLink> = {
  title: "Records/Worklist/Way back from a record",
  component: WorklistReturnLink,
  render: () => <OnTheRecord />,
};
export default meta;
type Story = StoryObj<typeof WorklistReturnLink>;

/** The way back reopens the drawer on the queue, scope and width it left. */
export const FromTheExpandedDrawer: Story = {
  play: async ({ canvasElement }) => {
    const back = await within(canvasElement).findByRole("link", {
      name: en["brief.queue.back"],
    });
    await expect(back).toHaveAttribute(
      "href",
      "#/home?filter=tasks&queue=1&queue_scope=team&wide=1",
    );
  },
};
