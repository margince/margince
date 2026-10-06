// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { type BoardDeal, DealCard } from "./composed";
import { EmailReference } from "./emailreference";
import { Eyebrow } from "./eyebrow";

// The card on its own, for the states the board canvases
// (pipelineboard.stories.tsx) cannot hold still: a flyout open under a settled
// pointer.
const meta: Meta<typeof DealCard> = {
  title: "Components/Text and data display/Deal card",
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
    companyId: "o-1",
    valueMinor: 33_000,
    currency: "EUR",
    ageMs: 9 * DAY_MS,
    ...extra,
  };
}

// Each state a card can be in, at the board's own column width: the slots stay
// level from card to card, and only a card that needs the reader carries
// colour. Every card is handed the verbs the deals board would give it, so an
// archived one gets the summary alone.
const verbs = {
  onSummary: () => undefined,
  onEmail: () => undefined,
  onAddTask: () => undefined,
};
const states: { label: string; deal: BoardDeal }[] = [
  {
    label: "Healthy",
    deal: boardDeal({
      name: "ERP connector, phase 2",
      closeDate: "2027-03-14",
      owner: { id: "u-2", name: "Jonas Ritter" },
      lastEmail: { agoMs: 2 * DAY_MS, direction: "inbound" },
    }),
  },
  {
    label: "A long imported name",
    deal: boardDeal({
      name: "deal-de-lumenhealth-x-gradion-dataplatform-migration-2026",
      valueMinor: 12_500_000,
      lastEmail: { agoMs: 5 * DAY_MS, direction: "outbound" },
    }),
  },
  {
    label: "Everything wrong",
    deal: boardDeal({
      stalled: true,
      singleThreaded: true,
      ageMs: 23 * DAY_MS,
      closeDate: "2027-04-30",
      closeDateProvisional: true,
      lastEmail: { agoMs: 23 * DAY_MS, direction: "outbound" },
    }),
  },
  {
    label: "Unpriced, unmailed",
    deal: boardDeal({ valueMinor: null, currency: null }),
  },
  { label: "Staged", deal: boardDeal({ staged: true }) },
  { label: "Archived", deal: boardDeal({ archived: true }) },
];

function StatesColumn() {
  return (
    <div
      style={{
        display: "grid",
        gap: "var(--space-3)",
        width: 300,
        maxWidth: "100%",
      }}
    >
      {states.map(({ label, deal }) => (
        <div key={label} style={{ display: "grid", gap: "var(--space-1)" }}>
          <Eyebrow>{label}</Eyebrow>
          <DealCard
            deal={{ ...deal, id: label }}
            href="#/deals/d2"
            zone="Europe/Berlin"
            actions={deal.archived ? { onSummary: verbs.onSummary } : verbs}
          />
        </div>
      ))}
    </div>
  );
}

export const BoardCardStates: StoryObj = { render: () => <StatesColumn /> };
export const BoardCardStatesDark: StoryObj = {
  globals: { theme: "dark" },
  render: () => <StatesColumn />,
};

// A card's mail line with the flyout a caller gave it: the last few subjects,
// read on hover, without opening the deal. The aside's content is the
// caller's — here the same stacked citations screens/dealmailaside.tsx reads
// from the timeline, handed in as fixtures — so what this canvas holds is the
// trigger, the panel and where it sits against the card.
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
          <div
            style={{ display: "grid", gap: "var(--space-2)", width: "18rem" }}
          >
            <Eyebrow>Previous emails</Eyebrow>
            <EmailReference
              subject="AW: Ausbildungsoffensive Bayern"
              occurredAt="Sent 10 d ago"
              stacked
            />
            <EmailReference
              subject="Ausbildungsoffensive Bayern, final"
              occurredAt="Sent 11 d ago"
              stacked
            />
            <EmailReference
              subject="AW: RetrieverClub MVP"
              occurredAt="Received 61 d ago"
              stacked
            />
          </div>
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
