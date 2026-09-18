// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { ProjectHealthModal } from "./projecthealthmodal";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Recording how a delivery is going, and correcting a reading that was wrong.
//
// One modal for both verbs, so the two frames that matter are the two titles
// and what each one seeds: a new reading starts on track and empty, a
// correction starts from what the reading actually said, and neither offers to
// change WHEN it was said.
//
// The third frame is the rule the form states instead of letting the server
// teach it through a refusal: anything but "on track" owes a note, and the
// save stays refused until one is there. It is reached by pressing the
// control, because that is the only way a reader reaches it.

const PROJECT = "33333333-3333-3333-3333-333333333333";
const READING = "77777777-7777-7777-7777-777777777701";

// Modal portals to document.body, so `#storybook-root` holds the preview
// decorator and nothing else however well the modal renders. Each frame drives
// a play that names what it expects; a rejecting play is a failure the render
// gate reports, which is what makes these stories worth their green.
const meta: Meta<typeof ProjectHealthModal> = {
  title: "Records/Project/Record a reading",
  component: ProjectHealthModal,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText("What is happening");
  },
};
export default meta;

type Story = StoryObj<typeof ProjectHealthModal>;

type Correcting = Parameters<typeof ProjectHealthModal>[0]["correcting"];

function modal(correcting?: Correcting) {
  return () => {
    // Recording and correcting are both writes, and neither is pressed here.
    // The stub is installed so a stray request cannot leave the iframe.
    installFetchStub({
      [`POST /projects/${PROJECT}/health-assessments`]: (body) =>
        jsonResponse(body),
    });
    return (
      <StoryProviders>
        <ProjectHealthModal
          open
          onClose={() => {}}
          projectId={PROJECT}
          correcting={correcting}
        />
      </StoryProviders>
    );
  };
}

/** A new reading: on track, no note owed, and the save already available. */
export const RecordingAReading: Story = { render: modal() };

/**
 * Correcting one that was wrong. The judgement and the words are already
 * there, so a reader fixing one sentence does not restate the whole reading,
 * and the caption says the correction keeps the original's effective time.
 */
export const CorrectingAReading: Story = {
  render: modal({
    id: READING,
    state: "at_risk",
    note: "Integration testing slipped a week.",
  }),
};

/**
 * The rule, in force: a delivery that is not on track owes an explanation, and
 * the save is refused until there is one. Pressed rather than posed, because
 * the modal opens on track and this is the state a reader walks into.
 */
export const OffTrackWithNoNoteYet: Story = {
  render: modal(),
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(
      await dialog.findByRole("button", { name: "Off track" }),
    );
    await dialog.findByText(
      "Say what is wrong, so whoever looks next does not have to guess.",
    );
  },
};
