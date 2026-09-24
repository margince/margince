// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { ConfirmSubmissionsPanel } from "./privacy.corrections";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// WHAT CONTACTS TYPED INTO THE LINK WE MAILED THEM, and who decides it.
//
// A correction is only reviewable as a COMPARISON — "she says Schmidt, we hold
// Schmitt" is the decision, and either half on its own is not — so every frame
// here is about whether the row carries both values, the field they are about,
// and the name of whoever proposed them.
//
// The second subject is the pair of rows that read almost alike and mean
// different things: a correction names a field and is accepted, a removal names
// none and can only be ACKNOWLEDGED, because what that contact asked for is a
// rights case answered somewhere else. The verb changes and nothing around it
// does, which is exactly the kind of difference a story shows and an assertion
// does not.

type ConfirmSubmission = components["schemas"]["ConfirmSubmission"];

// Typed rather than cast: a fixture that drops a required field still compiles
// under an assertion, and the story would go on drawing a shape the wire cannot
// send.
const MISSPELT_NAME: ConfirmSubmission = {
  id: "01a05500-0000-7000-8000-0000000000c1",
  contact_id: "01a05500-0000-7000-8000-0000000000aa",
  kind: "correction",
  field: "family_name",
  current_value: "Schmitt",
  proposed_value: "Schmidt",
  contact_name: "Anna Schmidt",
  submitted_at: "2026-08-01T09:00:00Z",
};

/** A contact asking to be removed sends no field and no value. */
const ASKED_TO_BE_REMOVED: ConfirmSubmission = {
  id: "01a05500-0000-7000-8000-0000000000c2",
  contact_id: "01a05500-0000-7000-8000-0000000000bb",
  kind: "removal",
  contact_name: "Bea Vogel",
  submitted_at: "2026-08-02T09:00:00Z",
};

function queue(rows: readonly ConfirmSubmission[]) {
  return { data: rows, page: { next_cursor: null, has_more: false } };
}

// The officer working this queue holds `contact:update`, because ACCEPTING a
// correction writes the contact named in it — and the seat ceiling is read
// before RBAC, so a read seat holding the same grant is refused every POST.
function panel(
  rows: readonly ConfirmSubmission[],
  extra: RouteMap = {},
  seat: "full" | "read" = "full",
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["update"] }, { seat }),
      "GET /confirm-submissions": () => jsonResponse(queue(rows)),
      ...extra,
    });
    return (
      <StoryProviders>
        <ConfirmSubmissionsPanel />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConfirmSubmissionsPanel> = {
  title: "Settings/Governance/Privacy and retention/Corrections",
  component: ConfirmSubmissionsPanel,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ConfirmSubmissionsPanel>;

/** The queue: a correction beside a removal, each with its own verb waiting. */
export const WaitingToBeDecided: Story = {
  render: panel([MISSPELT_NAME, ASKED_TO_BE_REMOVED]),
};

/**
 * One row opened for a decision: the note, then the two answers a hand apart.
 *
 * The note is offered on BOTH and matters most on the rejection — an accepted
 * correction explains itself, while "we did not change it" records no reason at
 * all, and the subject who asked is entitled to one when they ask again.
 */
export const DecidingOne: Story = {
  render: panel([MISSPELT_NAME, ASKED_TO_BE_REMOVED]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    // Every waiting row carries the verb, so the press names WHICH one: the
    // correction, because the pair of answers it opens is the subject here and
    // the removal's read differently. A bare role lookup finds two and rejects.
    const decide = await canvas.findAllByRole("button", { name: "Decide" });
    await userEvent.click(decide[0]);
  },
};

/**
 * A seat that may read the queue and answer none of it: the rows stand, the
 * comparison stands, and no row carries a verb that could only ever 403.
 */
export const ReadSeatDecidesNothing: Story = {
  render: panel([MISSPELT_NAME, ASKED_TO_BE_REMOVED], {}, "read"),
};

/** Nothing waiting — the honest empty state, not a blank panel. */
export const NothingWaiting: Story = { render: panel([]) };

/**
 * A FAILED READ IS NOT AN EMPTY QUEUE. Coercing an undefined answer to `[]` told
 * the reviewer nothing was waiting when the read had in fact failed, which is
 * the one wrong thing a work queue can say.
 */
export const TheQueueCouldNotBeRead: Story = {
  render: panel([], {
    "GET /confirm-submissions": () =>
      jsonResponse(
        {
          type: "https://errors.gradion.com/internal",
          title: "Internal Server Error",
          status: 500,
          code: "internal",
          detail: "The corrections queue could not be read.",
        },
        500,
      ),
  }),
};
