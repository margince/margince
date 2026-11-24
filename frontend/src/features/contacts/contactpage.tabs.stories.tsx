// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { ContactTab } from "../../lib/contacttab";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { populated } from "./contactpage.fixtures";
import { ContactRecordTabs } from "./contactpage.tabs";

// The contact's tab strip keeps the reader's place in the page pane.

const meta: Meta = {
  title: "Records/Contact 360/Tab strip",
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj;

function InPane() {
  const [tab, setTab] = useState<ContactTab>("overview");
  installFetchStub({ "GET /me": meRoute({ contact: ["read"] }) });
  return (
    <StoryProviders>
      <div
        data-testid="pane"
        className="scroll"
        style={{ display: "block", blockSize: "24rem", overflowY: "auto" }}
      >
        <div style={{ blockSize: "16rem" }}>{populated.contact.full_name}</div>
        <ContactRecordTabs view={populated} value={tab} onChange={setTab} />
        <div style={{ blockSize: "60rem" }}>{tab}</div>
      </div>
    </StoryProviders>
  );
}

// A tab pressed with the strip scrolled away raises the pane just enough to show the strip.
export const PressedOutOfView: Story = {
  render: () => <InPane />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const history = await canvas.findByRole("button", { name: /^History/ });
    const pane = canvas.getByTestId("pane");
    const row = canvasElement.querySelector(".recordtabs-row");
    if (!(row instanceof HTMLElement)) throw new Error("no tab strip drawn");
    const stripTop = () =>
      row.getBoundingClientRect().top - pane.getBoundingClientRect().top;
    pane.scrollTop += stripTop() + 200;
    await waitFor(() => expect(stripTop()).toBeLessThan(-150));
    history.focus({ preventScroll: true });
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(Math.abs(stripTop())).toBeLessThanOrEqual(1));
    await expect(history).toHaveAttribute("aria-pressed", "true");
    await expect(pane.scrollTop).toBeGreaterThan(0);
  },
};
