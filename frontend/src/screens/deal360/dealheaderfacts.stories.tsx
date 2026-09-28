// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { DealIdentityFacts } from "./dealheaderfacts";

// The facts beside the deal's name: what it is worth, where it sits on the
// board, when it is due, whose deal it is, which account it is on and how it
// reached Margince.
//
// The owner is the one that did not exist anywhere on this page before — not
// the header, not the rail, not the readings — so "whose deal is this" could
// only be answered by opening Edit.

const meta: Meta = {
  title: "Records/Deal 360",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const STAGES = [
  { id: "st-1", name: "Qualified" },
  { id: "st-2", name: "Proposal" },
];

export const Facts: Story = {
  render: () => {
    // The roster the owner name resolves through — the same read every
    // EntityRef in the app uses. Without it this story would document the
    // unresolved-owner fallback as the normal state.
    installFetchStub({
      // The strip asks the session who the reader IS, because the Source cell
      // can only say "entered by you" of a row the reader captured. Unrouted, the
      // probe answers loudly and the story documents a reader with no identity.
      "GET /me": meRoute({ deal: ["read"] }),
      "GET /users": () =>
        jsonResponse({
          data: [
            {
              id: "u-1",
              display_name: "Sofia Meier",
              email: "sofia@example.com",
            },
          ],
          page: { next_cursor: null },
        }),
    });
    return (
      <StoryProviders>
        <DealIdentityFacts
          deal={{
            amount_minor: 6_400_000,
            currency: "EUR",
            stage_id: "st-1",
            expected_close_date: "2026-09-30",
            owner_id: "u-1",
            source: "csv import",
            captured_by: "human:u-1",
          }}
          stages={STAGES}
          locale="en"
        />
      </StoryProviders>
    );
  },
};

/**
 * Unassigned, and the amount withheld from this reader.
 *
 * Both are stated rather than blank. An empty owner reads as a rendering
 * fault where "Unassigned" is a fact somebody can act on, and a bare dash for
 * the value would say "this deal is worth nothing" — a different claim from
 * "you may not see it".
 */
export const FactsWithheldAndUnassigned: Story = {
  render: () => {
    // A reader who may READ the deal and not its value: the grant is the same
    // one the story above carries, and what is withheld here rides on the
    // payload's `masked_fields` rather than on the session. No roster read —
    // an unassigned deal needs none to say so.
    installFetchStub({ "GET /me": meRoute({ deal: ["read"] }) });
    return (
      <StoryProviders>
        <DealIdentityFacts
          deal={{
            amount_minor: null,
            currency: "EUR",
            stage_id: "st-2",
            masked_fields: ["amount_minor"],
            source: "csv import",
            captured_by: "human:u-1",
          }}
          stages={STAGES}
          locale="en"
        />
      </StoryProviders>
    );
  },
};
