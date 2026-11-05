// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../../api/schema";
import { Button } from "../../../design-system/atoms";
import { StoryProviders } from "../../../testing/story-utils";
import { StateBanner } from "./dealroomdeck";

// The notice a room shows when buyers cannot simply walk in, one per state.
// The tab passes the way back as the action; the room's page passes none.

type DealRoom = components["schemas"]["DealRoom"];

const ROOM: DealRoom = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Nordwind — Fleet telematics",
  state: "live",
  source: "manual",
  captured_by: "u-me",
  version: 1,
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-24T14:02:00Z",
};

const meta: Meta = {
  title: "Records/Deal 360/Room state notice",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function notice(over: Partial<DealRoom>, action?: string) {
  return (
    <StoryProviders>
      <StateBanner
        room={{ ...ROOM, ...over }}
        action={action ? <Button>{action}</Button> : undefined}
      />
    </StoryProviders>
  );
}

export const Paused: Story = {
  render: () => notice({ state: "paused" }, "Resume"),
};

export const Closed: Story = {
  render: () => notice({ state: "closed" }, "Manage room"),
};

export const Expired: Story = {
  render: () => notice({ state: "expired" }, "Manage room"),
};

export const Archived: Story = {
  render: () => notice({ state: "archived" }),
};

export const LiveUntil: Story = {
  render: () => notice({ expires_at: "2026-12-31T23:59:59Z" }),
};

export const PausedDark: Story = {
  globals: { theme: "dark" },
  render: () => notice({ state: "paused" }, "Resume"),
};
