// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { BriefQueueItem } from "./briefqueue";
import { useBriefItemMark } from "./home.queries";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// One morning-brief entry, drawn and wired: the ranked deal, the five factors
// behind its score, and the three answers a reader can give it.
//
// Two surfaces draw this queue — Home reads it as the morning's narrative, the
// Worklist works through it — so the wiring around the presentational card
// (labels, formatters, the per-item pending and error projection) is what a
// second screen would otherwise copy. These stories are of that wiring.
//
// The evidence count is what the FIRST story is about: it reads through the
// reader's own plural rule now, so one evidence row says "1 evidence row"
// rather than "1 evidence rows".

const meta: Meta = {
  title: "Screens/Brief queue item",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Item = components["schemas"]["MorningBriefItem"];
type Deal = components["schemas"]["Deal"];

// A fixed instant, so "tomorrow at eight" is a stable string in the snooze verb
// rather than whatever the machine's clock says when the catalogue is built.
const NOW_MS = Date.parse("2026-08-20T09:00:00Z");

const deal = {
  id: "d-1",
  name: "Depot rollout — second site",
  amount_minor: 6_300_000,
  currency: "EUR",
  status: "open",
  version: 4,
} as unknown as Deal;

function item(over: Partial<Item>): Item {
  return {
    id: "bi-1",
    deal_id: "d-1",
    rank: 1,
    composite: 0.78,
    feature_vector: {
      winnability: 0.6,
      revenue: 0.9,
      timing: 0.7,
      momentum: 1,
      warmth: 0.45,
    },
    evidence_ids: ["ev-1", "ev-2", "ev-3"],
    state: "new",
    ...over,
  } as Item;
}

function Entry({
  subject,
  deals = [deal],
}: Readonly<{ subject: Item; deals?: Deal[] }>) {
  installFetchStub({});
  return (
    <StoryProviders>
      <Wired subject={subject} deals={deals} />
    </StoryProviders>
  );
}

// The mark mutation is the caller's, not the card's — a screen showing several
// entries runs ONE mutation across all of them, which is what lets the clicked
// card show the pending verb while its neighbours stay live. So the story owns
// it too, from inside the providers where the hook can run.
function Wired({
  subject,
  deals,
}: Readonly<{ subject: Item; deals: readonly Deal[] }>) {
  const mark = useBriefItemMark();
  return (
    <BriefQueueItem item={subject} deals={deals} nowMs={NOW_MS} mark={mark} />
  );
}

export const RankedFirst: Story = {
  render: () => <Entry subject={item({})} />,
};

// One evidence row: the singular arm of the plural pair, which is the wording a
// count comparison used to get wrong.
export const OneEvidenceRow: Story = {
  render: () => <Entry subject={item({ evidence_ids: ["ev-1"] })} />,
};

// Already answered. A dismissed item stays readable — the queue is a record of
// what was decided, not only of what is left.
export const Dismissed: Story = {
  render: () => (
    <Entry
      subject={item({
        state: "dismissed",
        state_at: "2026-08-20T08:40:00Z",
      })}
    />
  ),
};

export const SnoozedUntilTomorrow: Story = {
  render: () => (
    <Entry
      subject={item({
        state: "snoozed",
        state_at: "2026-08-20T08:40:00Z",
        snoozed_until: "2026-08-21T06:00:00Z",
      })}
    />
  ),
};

// The deal behind the item is not in the page of deals this screen read. The
// card says what it can — rank, factors, evidence — and names no deal rather
// than inventing one.
export const DealNotOnThisPage: Story = {
  render: () => <Entry subject={item({})} deals={[]} />,
};

// A refused mark. The error belongs to the item that was clicked, so a queue of
// several entries shows it on one card and leaves the rest alone — which is why
// the story CLICKS rather than posing the state: the projection from the
// mutation's variables to this card is the part worth seeing work.
export const MarkRefused: Story = {
  // The refusal is the subject: the 409 this story is about reaches the console
  // as a failed request, so the story says it expects one rather than the run
  // being told to ignore all of them.
  tags: ["uat-expected-console-error"],
  render: () => {
    installFetchStub({
      "POST /brief/items/bi-1/act": () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "The item changed",
            status: 409,
            code: "version_skew",
            detail: "This item was already answered on another device.",
          },
          409,
        ),
    });
    return (
      <StoryProviders>
        <Wired subject={item({})} deals={[deal]} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("button", { name: "Done" }));
    await canvas.findByText(/already answered/);
  },
};
