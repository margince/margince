// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../../api/schema";
import { ProblemError } from "../../../lib/problem";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../../testing/story-utils";
import { ConfirmAdvanceModal } from "./confirmadvance";

// The confirm a terminal stage move goes through. A loss asks for a reason
// and for what happens to the open tasks. A win with a signed contract is one
// press. A win without one asks how it was won first.
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

// Two open tasks on the deal, the shape the close asks about.
const OPEN_TASKS = {
  data: [
    {
      id: "t-invoice",
      kind: "task",
      subject: "Send the final invoice",
      is_done: false,
      version: 2,
      occurred_at: "2026-10-01T09:00:00Z",
    },
    {
      id: "t-device",
      kind: "task",
      subject: "Return the loan device",
      is_done: false,
      version: 1,
      occurred_at: "2026-10-02T09:00:00Z",
    },
  ],
  page: { has_more: false },
};

function advance(
  args: Readonly<{
    stage: Stage;
    onConfirm: () => Promise<unknown>;
    tasks?: unknown;
    // Whether the deal has a signed contract. Left out, the read answers
    // nothing usable and the dialog asks only if the server refuses.
    signed?: boolean;
  }>,
) {
  return () => {
    // The dot beside the title reads the agent-tool tiers. The deal's tasks
    // feed the question about them, and a win reads its signed contract.
    installFetchStub({
      "GET /agent-tools": () =>
        jsonResponse({ data: [{ name: "progress_deal", tier: "confirm" }] }),
      "GET /activities": () =>
        jsonResponse(args.tasks ?? { data: [], page: { has_more: false } }),
      "GET /loss-reasons": () => jsonResponse({ data: LOSS_REASONS }),
      ...(args.signed === undefined
        ? {}
        : {
            "GET /deals/deal-1/win-evidence": () =>
              jsonResponse({ has_signed_contract: args.signed }),
          }),
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

// The list a loss is picked from. A retired reason is not offered.
const LOSS_REASONS = [
  ["price", "Price", true],
  ["competitor", "Chose a competitor", true],
  ["timing", "Bad timing", false],
].map(([key, label, active]) => ({
  id: `reason-${key}`,
  key,
  label,
  active,
  sort_order: 10,
  system: true,
  version: 1,
  created_at: "2026-10-01T08:00:00Z",
  updated_at: "2026-10-01T08:00:00Z",
}));

const meta: Meta<typeof ConfirmAdvanceModal> = {
  title: "Records/Deal 360/Confirm advance",
  component: ConfirmAdvanceModal,
};
export default meta;

type Story = StoryObj<typeof ConfirmAdvanceModal>;

/** A loss: the reason picker and the note are on the dialog, and Confirm waits for a reason. */
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
    signed: true,
  }),
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText(/closes the deal as won/);
    await waitFor(() =>
      expect(dialog.getByRole("button", { name: "Confirm" })).toBeEnabled(),
    );
  },
};

const winWithoutContract = advance({
  stage: terminal("Won", "won"),
  onConfirm: () => Promise.resolve(SAVED),
  signed: false,
});

// A win on a deal with no signed contract. The question is there from the
// start, and Confirm waits for the answer instead of being refused for it.
async function askedBeforeConfirm() {
  const dialog = within(await screen.findByRole("dialog"));
  await dialog.findByRole("combobox", { name: "How was it won?" });
  expect(
    dialog.getByRole("button", { name: "Confirm" }),
  ).toHaveAccessibleDescription("Pick how the deal was won.");
}

export const WinWithoutContract: Story = {
  render: winWithoutContract,
  play: askedBeforeConfirm,
};

export const WinWithoutContractDark: Story = {
  globals: { theme: "dark" },
  render: winWithoutContract,
  play: askedBeforeConfirm,
};

const lostWithTasks = advance({
  stage: terminal("Lost", "lost"),
  onConfirm: () => Promise.resolve(SAVED),
  tasks: OPEN_TASKS,
});

// A loss on a deal with open tasks: they are listed, and Confirm waits for
// Complete all or Keep as well as for the reason.
async function askedAboutTheTasks() {
  const dialog = within(await screen.findByRole("dialog"));
  await dialog.findByText("Send the final invoice");
  await dialog.findByRole("radio", { name: /Complete all/ });
}

export const ClosingLostWithOpenTasks: Story = {
  render: lostWithTasks,
  play: askedAboutTheTasks,
};

export const ClosingLostWithOpenTasksDark: Story = {
  globals: { theme: "dark" },
  render: lostWithTasks,
  play: askedAboutTheTasks,
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
  // Confirm takes a press once the deal's open tasks have been read.
  await waitFor(() =>
    expect(dialog.getByRole("button", { name: "Confirm" })).toBeEnabled(),
  );
  await userEvent.click(dialog.getByRole("button", { name: "Confirm" }));
  await dialog.findByText(/Record how the deal was won/);
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
