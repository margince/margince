// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { deckItems } from "./brief";
import { DecisionsSection } from "./brief.decisions";
import { bundle, proposal, singles } from "./brief.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// WAITING ON YOU — the zone, and what the redesign gave it.
//
// Three things to look at in every frame: the panel's header band, which is the
// same band Today wears, with the Deck/List switch in it; one LINE per decision
// with the proposal behind the line's own control; and three rows before the
// zone hands the reader on to the approvals lane.
//
// Read every frame in BOTH themes. The rows sit on `.staging-card`'s dashed
// `--aiMed` edge over `--aiLight` — the product's one signal that what is on a
// surface is PROPOSED rather than recorded — and both re-resolve on the flip.
//
// The clock is a fixed instant: a countdown chip read off the machine's clock
// would say something different every time the catalog was opened.

const NOW = Date.parse("2026-08-20T06:00:00Z");

// THE READS THIS ZONE MAKES, answered honestly.
//
// `GET /me` is not optional furniture: the card's provenance reading needs the
// signed-in reader's own id to tell "an agent staged this" from "you did", and a
// refused session reads as a malformed one — every grant then fails closed and
// the surface draws a posture no rep has.
//
// The grants are what these frames are ABOUT, spelled rather than guessed:
// there is no `approval` object in the RBAC vocabulary, because a proposal is
// gated by the act it proposes. These are the two acts the fixtures stage — a
// mail to send and a deal to move — so this is the seat that can answer them.
const ROUTES: RouteMap = {
  "GET /me": meRoute(
    { activity: ["read", "create"], deal: ["read", "update"] },
    { roles: ["rep"] },
  ),
  // The autonomy dot's own source. Routed rather than left to the fallback: an
  // empty catalog is a real state and it draws the CONFIRM dot for every kind,
  // so a frame that let it fall through would picture the one reading these
  // proposals are not — a tool nobody has cleared to act on its own.
  "GET /agent-tools": () =>
    jsonResponse({
      data: [
        { name: "send_email", tier: "confirmation_required" },
        { name: "progress_deal", tier: "auto_execute" },
      ],
    }),
};

const meta: Meta<typeof DecisionsSection> = {
  title: "Shell/Brief decisions",
  component: DecisionsSection,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      installFetchStub(ROUTES);
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;

type Story = StoryObj<typeof DecisionsSection>;

const BASE = {
  nowMs: NOW,
  state: "ready" as const,
  onAlreadyDecided: () => undefined,
};

/** Nine things to decide, which is more than the zone draws. */
const MANY = deckItems(
  Array.from({ length: 9 }, (_, at) =>
    proposal(`ap-${at}`, `Send the follow-up to contact ${at + 1}`, {
      expires_at: "2026-08-20T12:00:00Z",
    }),
  ),
);

// The default: a list of lines, three of them, and the way to the rest. What to
// check is the density — the question, its chips and its verbs at one x down
// the column, so three rows read as three questions rather than three cards.
export const List: Story = {
  args: { ...BASE, items: MANY },
};

// One row open. The proposal is in a popover beside the line, so the two rows
// under it have not moved — which is why this is not a fold.
export const RowOpen: Story = {
  args: { ...BASE, items: MANY },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const triggers = await canvas.findAllByRole("button", {
      name: "What is being proposed",
    });
    await userEvent.click(triggers[0]);
  },
};

// The heavier verdicts, out of the menu. A rejection and an edit each get a
// whole line to say what they do, which is the trade a glyph cannot make.
export const OtherAnswers: Story = {
  args: { ...BASE, items: MANY },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const menus = await canvas.findAllByRole("button", {
      name: "Other answers",
    });
    await userEvent.click(menus[0]);
  },
};

// Behind the switch: the deck, unchanged. One decision at a time, the whole
// payload on the plate, and the tray under it.
export const Deck: Story = {
  args: { ...BASE, items: MANY },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("button", { name: "Deck" }));
  },
};

// A bundle among singles. One act staged ten proposals, the API decides them in
// one call, and the line therefore asks ONE question with its members behind an
// expander in the proposal.
export const WithABundle: Story = {
  args: { ...BASE, items: deckItems([...singles, ...bundle]) },
};

// Nobody is blocked on the reader. The switch goes with the rows it switched
// between: a control with nothing behind it is noise on the one surface whose
// whole point is that there is nothing left to do.
export const NothingWaiting: Story = {
  args: { ...BASE, items: [] },
};

export const Loading: Story = {
  args: { ...BASE, items: [], state: "loading" },
};

// The read failed. The zone says so where the rows would be — "nothing is
// waiting on you" over a failed read is the one thing this surface must not
// say.
export const ReadFailed: Story = {
  args: { ...BASE, items: [], state: "failed" },
};
