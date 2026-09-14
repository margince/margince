// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { type BoardDeal, DealCard } from "./composed";

// The card on its own, for the states the board canvases (composed.stories.tsx)
// cannot hold still: a flyout open under a settled pointer.
const meta: Meta<typeof DealCard> = {
  title: "Design System/DealCard",
  component: DealCard,
  parameters: { layout: "padded" },
};
export default meta;

const DAY_MS = 24 * 60 * 60 * 1000;

function boardDeal(extra?: Partial<BoardDeal>): BoardDeal {
  return {
    id: "d2",
    name: "Fabrikam expansion",
    company: "Acme GmbH",
    valueMinor: 33_000,
    currency: "EUR",
    ageMs: 9 * DAY_MS,
    ...extra,
  };
}

// A card's mail line with the flyout a caller gave it: the last few subjects,
// read on hover, without opening the deal. The aside's content is the
// caller's — here a static list standing in for what screens/dealmailaside.tsx
// reads from the timeline — so what this canvas holds is the trigger, the
// panel and where it sits against the card.
export const BoardCardMailFlyout: StoryObj = {
  render: () => (
    <div style={{ maxWidth: 280 }}>
      <DealCard
        deal={boardDeal({
          stalled: true,
          closeDate: "2026-09-30",
          lastEmail: { agoMs: 10 * DAY_MS, direction: "outbound" },
        })}
        href="#/deals/d2"
        zone="Europe/Berlin"
        mailAside={() => (
          <ul style={{ margin: 0, paddingLeft: "1rem" }}>
            <li>AW: Ausbildungsoffensive Bayern — sent 10 d ago</li>
            <li>Ausbildungsoffensive Bayern, final — sent 11 d ago</li>
            <li>AW: RetrieverClub MVP — received 2 months ago</li>
          </ul>
        )}
      />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.hover(canvas.getByRole("button", { name: /Last email/ }));
    await waitFor(() =>
      expect(document.body.querySelector(".popover-panel")).toBeInTheDocument(),
    );
  },
};
