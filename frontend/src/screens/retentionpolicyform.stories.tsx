// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import { Modal } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import { RetentionPolicyForm } from "./retentionpolicyform";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";
import "./retention.css";

// THE AUTHORING FORM for a retention rule — four inputs committed together.
//
// It brings no surface of its own on purpose: it is the BODY of the dialog the
// ladder's "Add policy" row opens, and a card inside a dialog is a box in a box.
// So the frames mount it in that `Modal`, under the ladder's own heading. The
// dialog portals to the document body, which is where the plays below query it.
//
// The frames are about the two refusals, because the resting form is the easy
// half. A window that is not a whole number is answered by the field holding it
// and never by the commit; a scope another admin took since this list was
// fetched is answered by the server, and the form carries that refusal whatever
// the select offered — which is why the select offers the WHOLE authorable enum
// rather than the unused half of it.

const ADD_TITLE = "retention-add-policy-title";

function Dialog({ onDone }: Readonly<{ onDone: () => void }>) {
  const t = useT();
  return (
    <Modal open onClose={onDone} labelledBy={ADD_TITLE}>
      <Heading size="large" id={ADD_TITLE} className="t-h2 modal-title">
        {t("retention.addPolicy")}
      </Heading>
      <RetentionPolicyForm onDone={onDone} />
    </Modal>
  );
}

function form(routes: RouteMap = {}) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <Dialog onDone={fn()} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof RetentionPolicyForm> = {
  title: "Settings/Governance/Privacy & retention/Retention policy form",
  component: RetentionPolicyForm,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof RetentionPolicyForm>;

// The scope this form was about was taken since the ladder was fetched.
const duplicate = form({
  "POST /retention-policies": () =>
    jsonResponse(
      {
        type: "https://errors.gradion.com/conflict",
        title: "Conflict",
        status: 409,
        code: "conflict",
        detail:
          'retention policy for scope "lead/unconverted" already exists: conflict',
      },
      409,
    ),
});

// Spelled once and used by both refusal frames: they differ in the THEME the
// sentence is drawn in, and a second copy of the gesture is a second thing to
// fix when the commit is renamed. The dialog portals to the document body, so
// everything after the open is queried where it actually renders.
const commitAndBeRefused: Story["play"] = async () => {
  const dialog = within(await screen.findByRole("dialog"));
  await userEvent.type(await dialog.findByLabelText(/window in days/i), "365");
  await userEvent.click(dialog.getByRole("button", { name: /create policy/i }));
  await dialog.findByRole("alert");
};

/**
 * At rest, defaulting to ARCHIVE.
 *
 * The least destructive action is the one a new policy starts on, so an operator
 * who never touches that field cannot author an erase by omission. The commit is
 * refused until a window is given: there is no sensible default number of days,
 * and guessing one would author a rule nobody chose.
 */
export const AuthoringANewPolicy: Story = {
  render: form({
    "POST /retention-policies": () => jsonResponse({ id: "p-1" }, 201),
  }),
};

/**
 * A WINDOW THAT IS NOT A NUMBER, answered by the field that holds it. The hint
 * appears only once something has been typed — an empty field is not yet wrong —
 * and the commit stays refused while it stands, so there is no press that could
 * send the server a window it would have to reject.
 */
export const TheWindowIsNotAWholeNumber: Story = {
  render: form(),
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.type(
      await dialog.findByLabelText(/window in days/i),
      "soon",
    );
  },
};

/**
 * THE SCOPE IS ALREADY TAKEN: a 409 with exactly one cause.
 *
 * Uniqueness is the database's answer rather than this form's, so the refusal
 * names the row to edit instead of relaying the constraint — and the ladder
 * behind the dialog is re-read at the same time, because a message naming a row
 * the list has never shown would send the operator looking for nothing.
 */
export const TheScopeIsAlreadyTaken: Story = {
  render: duplicate,
  play: commitAndBeRefused,
};

/**
 * The same refusal in dark, where the only thing carrying it is an ink.
 *
 * The sentence is `--dangerText` on the dialog's own elevated ground with no
 * tint, no border and no icon beside it — the words are the signal — so this is
 * the frame that says whether a refusal still reads as one when the ground under
 * it inverts.
 */
export const TheScopeIsAlreadyTakenDark: Story = {
  render: duplicate,
  play: commitAndBeRefused,
  globals: { theme: "dark" },
};
