/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { deckItems } from "../../lib/brief.decisions.items";
import { proposal, render, stubApi } from "../../testing/brief.testkit";
import { DecisionsSection } from "./brief.decisions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("shows the stages a proposal-stage question skips, in the deck", async () => {
  stubApi({});
  const user = userEvent.setup();
  const queue = [
    proposal(
      "move-1",
      "A proposal went out on Hanoi depot. Move the deal to “Proposal”?",
      {
        kind: "deal_proposal_stage",
        proposed_by: "system:proposal-stage",
        proposed_change: {
          to_stage_name: "Proposal",
          skipped_stage_names: ["Qualified", "Discovery"],
        },
      },
    ),
  ];
  render(
    <DecisionsSection
      items={deckItems(queue)}
      nowMs={Date.parse("2026-11-20T10:00:00Z")}
      state="ready"
      onAlreadyDecided={() => undefined}
    />,
  );
  await user.click(
    screen.getByRole("button", { name: en["brief.deck.viewDeck"] }),
  );
  expect(
    await screen.findByText(en["approval.field.skipped_stages"]),
  ).toBeTruthy();
  expect(screen.getByText("Qualified, Discovery")).toBeTruthy();
});
