// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { useLocale, useT } from "../../i18n";
import { en } from "../../i18n/en";
import { company360 } from "../../testing/company.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { WITHHELD_READING } from "./company360health";
import { LastTouchStat } from "./company360lasttouch";

type Company360 = components["schemas"]["Company360"];
type LastContact = NonNullable<Company360["last_contact"]>;

// Fourteen hours ahead of UTC, so noon UTC is already the next day here. A
// day entry still reads its own date; an instant reads this zone's.
const RECORD_ZONE = "Pacific/Kiritimati";

function Tile({
  lastContact,
  withheld = false,
}: Readonly<{ lastContact?: LastContact; withheld?: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <LastTouchStat
      view={{
        ...company360,
        as_of: "2026-08-14T09:00:00Z",
        last_contact: lastContact,
      }}
      withheld={withheld}
      locale={locale}
      recordZone={RECORD_ZONE}
      t={t}
    />
  );
}

const meta = {
  title: "Records/Company 360/Last contact",
  component: Tile,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof Tile>;
export default meta;
type Story = StoryObj<typeof meta>;

const call: LastContact = {
  at: "2026-08-12T08:00:00Z",
  occurred_precision: "instant",
  kind: "call",
  activity_id: "0198f011-eeee-7000-8000-000000000001",
};

export const Instant: Story = { args: { lastContact: call } };

// A meeting logged by its date alone, the day before the read. The tile
// counts from that date and names it with no clock, whatever the zone.
export const DayOnly: Story = {
  args: {
    lastContact: {
      ...call,
      at: "2026-08-13T12:00:00Z",
      occurred_precision: "day",
      kind: "meeting",
    },
  },
  play: async ({ canvasElement }) => {
    const label = await within(canvasElement).findByText(
      en["co.strip.lastTouch"],
    );
    const tile = label.closest(".stat-card");
    await expect(tile).not.toBeNull();
    await expect(tile?.textContent).toMatch(/13 Aug 2026/);
    await expect(tile?.textContent).toMatch(/1 d/);
    await expect(tile?.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};

export const Never: Story = { args: {} };

export const Withheld: Story = {
  args: { lastContact: call, withheld: true },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText(en[WITHHELD_READING]),
    ).toBeVisible();
  },
};
