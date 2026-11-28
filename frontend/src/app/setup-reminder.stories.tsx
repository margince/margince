// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import { en as englishCopy } from "../i18n/en";
import { parseHash } from "../lib/router";
import {
  gmail,
  type SetupWorld,
  setupReminderGrants,
  setupReminderRoutes,
} from "../testing/setupreminder.fixtures";
import { StoryProviders, stubWithSession } from "../testing/story-utils";
import { TopBar } from "./topbar";

// The top bar's "Finish setup" trigger and the checklist behind it, in the
// world `setupreminder.fixtures.ts` describes.

function reminder(world: SetupWorld = {}) {
  stubWithSession(setupReminderRoutes(world), setupReminderGrants(world));
  return (
    <StoryProviders>
      <div className="app">
        <main className="main">
          <TopBar
            route={parseHash("#/settings")}
            onOpenSearch={() => undefined}
          />
        </main>
      </div>
    </StoryProviders>
  );
}

const meta = {
  title: "Shell/Setup reminder",
  render: () => reminder(),
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const openPanel: Story["play"] = async () => {
  await userEvent.click(
    await screen.findByRole("button", { name: /Finish setup/ }),
  );
};

export const Closed: Story = {};
export const ClosedDark: Story = { globals: { theme: "dark" } };
/** Open, on the content card's right and top edges like every panel the strip opens. */
export const Open: Story = {
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: /Finish setup/ }),
    );
    const panel = await screen.findByRole("heading", {
      name: englishCopy["setup.reminder.title"],
    });
    const pop = panel.closest(".popover-panel");
    if (!pop) {
      throw new Error("the panel is missing");
    }
    await Promise.all(
      pop.getAnimations({ subtree: true }).map((a) => a.finished),
    );
    const root = getComputedStyle(document.documentElement);
    const cardInset = Number.parseFloat(root.getPropertyValue("--space-2"));
    const rect = pop.getBoundingClientRect();
    await expect(window.innerWidth - rect.right).toBe(cardInset);
    await expect(rect.top).toBe(
      Number.parseFloat(root.getPropertyValue("--topbarH")),
    );
  },
};
export const OpenDark: Story = { play: openPanel, globals: { theme: "dark" } };
// A phone's bar has no room for the trigger, so the account menu carries it.
// The row follows Help and feedback and opens the next step's page.
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: /account$/i }),
    );
    const row = await screen.findByRole("menuitem", { name: /Finish setup/ });
    await expect(row).toHaveAttribute("href", "/#/settings/connections");
    await expect(
      screen.queryByRole("button", { name: /Finish setup/ }),
    ).toBeNull();
  },
};

// No company app yet: mail waits on an administrator, so no step offers a verb.
export const Blocked: Story = {
  render: () => reminder({ rows: [], reason: "app_missing" }),
  play: openPanel,
};

// An expired calendar: the current step's one primary verb is Reconnect.
export const NeedsReconnect: Story = {
  render: () =>
    reminder({
      rows: [
        gmail,
        {
          ...gmail,
          id: "00000000-0000-4000-8000-000000000012",
          provider: "gcal",
          status: "reauth_required",
        },
      ],
    }),
  play: openPanel,
};

// The default world opened: booking shows the grey Waiting badge.
export const Waiting: Story = { play: openPanel };

// Mail, voice and booking done: connecting an AI tool is the one step left.
export const AiToolLeft: Story = {
  render: () =>
    reminder({
      rows: [
        gmail,
        {
          ...gmail,
          id: "00000000-0000-4000-8000-000000000012",
          provider: "gcal",
          scopes: ["https://www.googleapis.com/auth/calendar.events"],
        },
      ],
      finished: true,
      aiConnected: false,
    }),
  play: openPanel,
};

// The admin skipped the AI key on first run, so setting it up leads the list.
// Its verb opens Settings, AI models, where the same form waits.
export const AiKeySkipped: Story = {
  render: () => reminder({ modelUnbound: true }),
  play: openPanel,
};

// Every step done: the reminder stands down and the bar carries no trigger.
export const AllDone: Story = {
  render: () =>
    reminder({
      rows: [
        gmail,
        {
          ...gmail,
          id: "00000000-0000-4000-8000-000000000012",
          provider: "gcal",
          scopes: ["https://www.googleapis.com/auth/calendar.events"],
        },
      ],
      finished: true,
    }),
};
