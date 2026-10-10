// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { Panel, PanelBody, PanelIntro } from "./panel";
import { PanelNotices } from "./panelnotices";

// The band under a card's rows: the reader's read-only posture and the card's
// last refused write, each said once for the whole card.
const meta: Meta<typeof PanelNotices> = {
  title: "Components/Messaging/Panel notices",
  component: PanelNotices,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Panel title="Lead sources">
          <PanelBody>
            <PanelIntro>Where a lead came from.</PanelIntro>
          </PanelBody>
          <Story />
        </Panel>
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof PanelNotices>;

const REFUSED = {
  title: "Not saved",
  error: new Error("network down"),
};

export const ReadOnly: Story = {
  args: { readOnly: "You can view this list but not change it." },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("heading", { name: "Lead sources" });
    await expect(
      canvas.getByText("You can view this list but not change it."),
    ).toBeVisible();
  },
};

export const RefusedWrite: Story = {
  args: { refused: REFUSED },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("heading", { name: "Lead sources" });
    await expect(canvas.getByText("Not saved")).toBeVisible();
  },
};

export const Both: Story = {
  args: {
    readOnly: "You can view this list but not change it.",
    refused: REFUSED,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("heading", { name: "Lead sources" });
    await expect(canvas.getByText("Not saved")).toBeVisible();
    await expect(
      canvas.getByText("You can view this list but not change it."),
    ).toBeVisible();
  },
};

// A reader who may write, with nothing refused: the band draws nothing.
export const NothingToSay: Story = {
  args: {},
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("heading", { name: "Lead sources" });
    await expect(canvasElement.querySelector(".panel-notices")).toBeNull();
  },
};
