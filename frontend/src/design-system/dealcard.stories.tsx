// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { lastEmailAgo } from "../testing/dealmail";
import { DealCard } from "./dealcard";
import { EmailReference } from "./emailreference";
import { Eyebrow } from "./eyebrow";
import type { BoardDeal } from "./pipelineboard";

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
    idleSince: new Date(
      Date.parse("2026-09-15T12:00:00Z") - 9 * DAY_MS,
    ).toISOString(),
    measuredAt: Date.parse("2026-09-15T12:00:00Z"),
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
      lastEmail: lastEmailAgo(2 * DAY_MS, "inbound"),
    }),
  },
  {
    label: "A long imported name",
    deal: boardDeal({
      name: "deal-de-lumenhealth-x-gradion-dataplatform-migration-2026",
      valueMinor: 12_500_000,
      lastEmail: lastEmailAgo(5 * DAY_MS, "outbound"),
    }),
  },
  {
    label: "Everything wrong",
    deal: boardDeal({
      stalled: true,
      singleThreaded: true,
      idleSince: new Date(
        Date.parse("2026-09-15T12:00:00Z") - 23 * DAY_MS,
      ).toISOString(),
      measuredAt: Date.parse("2026-09-15T12:00:00Z"),
      closeDate: "2027-04-30",
      closeDateProvisional: true,
      lastEmail: lastEmailAgo(23 * DAY_MS, "outbound"),
    }),
  },
  {
    // The widest mail chip: a date-only email from another year.
    label: "Mail dated last year",
    deal: boardDeal({
      stalled: true,
      singleThreaded: true,
      idleSince: new Date(
        Date.parse("2026-09-15T12:00:00Z") - 23 * DAY_MS,
      ).toISOString(),
      measuredAt: Date.parse("2026-09-15T12:00:00Z"),
      lastEmail: {
        at: "2025-09-14T12:00:00Z",
        precision: "day",
        measuredAt: Date.parse("2026-09-15T12:00:00Z"),
        direction: "outbound",
      },
    }),
  },
  {
    label: "The same calendar age on both badges",
    deal: boardDeal({
      stalled: true,
      idleSince: "2025-01-01T22:30:00Z",
      measuredAt: Date.parse("2025-03-12T10:00:00Z"),
      lastEmail: {
        at: "2025-01-01T22:30:00Z",
        precision: "instant",
        measuredAt: Date.parse("2025-03-12T10:00:00Z"),
        direction: "inbound",
      },
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
// caller's. Here it is the stacked citations features/deals/dealmailaside.tsx
// reads from the timeline, handed in as fixtures. So this canvas holds the
// trigger, the panel and where it sits against the card.
export const BoardCardMailFlyout: StoryObj = {
  render: () => (
    <div style={{ maxWidth: 280 }}>
      <DealCard
        deal={boardDeal({
          stalled: true,
          closeDate: "2026-09-30",
          lastEmail: lastEmailAgo(10 * DAY_MS, "outbound"),
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

export const CalendarAgeAgreement: StoryObj = {
  render: () => (
    <DealCard
      deal={boardDeal({
        stalled: true,
        idleSince: "2025-01-01T22:30:00Z",
        measuredAt: Date.parse("2025-03-12T10:00:00Z"),
        lastEmail: {
          at: "2025-01-01T22:30:00Z",
          precision: "instant",
          measuredAt: Date.parse("2025-03-12T10:00:00Z"),
          direction: "inbound",
        },
      })}
      href="#/deals/d2"
      zone="Europe/Berlin"
    />
  ),
};
