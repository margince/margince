// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { ProblemError } from "../common";
import { installFetchStub, jsonResponse, StoryProviders } from "../story-utils";
import { ConfirmAdvanceModal } from "./confirmadvance";

// The confirm a terminal stage move goes through, in the three shapes it
// takes: a loss, which asks for a reason; an ordinary win, which is one press;
// and the win the server refuses for naming no evidence, which is the only
// refusal that keeps this dialog open rather than closing it.
//
// `Modal` portals to document.body, so the story canvas holds nothing and the
// dialog is queried off the document. Each play is what proves the dialog drew
// what its story is named for.

type Stage = components["schemas"]["Stage"];

function terminal(name: string, semantic: Stage["semantic"]): Stage {
  return {
    id: `stage-${semantic}`,
    pipeline_id: "pl-1",
    name,
    position: 4,
    semantic,
    win_probability: semantic === "won" ? 100 : 0,
  };
}

// The saved deal as this dialog reads it: an id and a version are what tell a
// landed close from a refusal (`isSavedDeal`), and nothing else here is read.
const SAVED = { id: "deal-1", version: 4 };

// What the server answers a win that names neither a signed contract nor a
// reason. A ProblemError rather than a bare body, because the dialog keys on
// the FIELD CODE and only a ProblemError carries one — an advance refused for
// any other cause closes the dialog instead.
const WIN_EVIDENCE_REFUSAL = new ProblemError({
  code: "validation_error",
  title: "Unprocessable Entity",
  status: 422,
  details: {
    errors: [
      {
        field: "won_without_contract_reason",
        code: "win_evidence_required",
        message: "a won deal needs a contract, or a reason there is none",
      },
    ],
  },
});

function advance(
  args: Readonly<{ stage: Stage; onConfirm: () => Promise<unknown> }>,
) {
  return () => {
    // The autonomy dot beside the title reads the agent-tool tiers; nothing
    // else on this dialog asks the network.
    installFetchStub({
      "GET /agent-tools": () =>
        jsonResponse({ data: [{ name: "progress_deal", tier: "confirm" }] }),
    });
    return (
      <StoryProviders>
        <ConfirmAdvanceModal
          pending={{ dealId: "deal-1", version: 3, toStage: args.stage }}
          onClose={() => {}}
          onConfirm={args.onConfirm}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConfirmAdvanceModal> = {
  title: "Records/Deal 360/Confirm advance",
  component: ConfirmAdvanceModal,
};
export default meta;

type Story = StoryObj<typeof ConfirmAdvanceModal>;

/** A loss: the reason field is on the dialog, and Confirm waits for it. */
export const ClosingLost: Story = {
  render: advance({
    stage: terminal("Lost", "lost"),
    onConfirm: () => Promise.resolve(SAVED),
  }),
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText(/closes the deal as lost/);
  },
};

/** A win with paper behind it: one press, and no question asked. */
export const ClosingWon: Story = {
  render: advance({
    stage: terminal("Won", "won"),
    onConfirm: () => Promise.resolve(SAVED),
  }),
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText(/closes the deal as won/);
  },
};

const refusedWin = advance({
  stage: terminal("Won", "won"),
  onConfirm: () => Promise.resolve(WIN_EVIDENCE_REFUSAL),
});

// The dialog after the server refused THIS deal's win for naming no evidence:
// it stays open and grows the reason vocabulary, because that is a question
// the reader can answer right here.
async function askedHowItWasWon() {
  const dialog = within(await screen.findByRole("dialog"));
  await userEvent.click(dialog.getByRole("button", { name: "Confirm" }));
  await dialog.findByText(/tell us how it was won/);
}

export const WinNamingNoEvidence: Story = {
  render: refusedWin,
  play: askedHowItWasWon,
};

// The same refusal on the dark ground: the fields it grows appear inside an
// already-open dialog, where the surface tint is a lifted derivation.
export const WinNamingNoEvidenceDark: Story = {
  globals: { theme: "dark" },
  render: refusedWin,
  play: askedHowItWasWon,
};
