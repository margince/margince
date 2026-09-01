// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { IntroRequestModal } from "./introrequest";

// The dialog that turns a route into a move.
//
// Its three states are the three answers the endpoint gives: not yet asked, a
// draft to take away, and a refusal. The middle one is what a reader spends
// their time in, which is why the draft is editable rather than read-only.

const meta = {
  title: "Records/Company 360/People/Intro request",
  component: IntroRequestModal,
  parameters: { layout: "padded" },
} satisfies Meta<typeof IntroRequestModal>;

export default meta;
type Story = StoryObj<typeof meta>;

const TARGET = {
  personId: "p-1",
  personName: "Philipp Königs",
  viaUserId: "u-1",
  viaName: "Sofia Meier",
};

/** Answers every draft request with the same message. */
function story(draft: unknown, status = 200) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ organization: ["read"], person: ["read"] }),
      "POST /organizations/o-1/intro-request-draft": () =>
        jsonResponse(draft, status),
    });
    return (
      <StoryProviders>
        <IntroRequestModal
          orgId="o-1"
          target={TARGET}
          dealId="d-1"
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

/** Before anything is written: who is being asked, and about whom. */
export const BeforeWriting: Story = {
  args: { orgId: "o-1", target: TARGET, onClose: () => {} },
  render: story({}),
};

/**
 * A model wrote it, and the tag says so. Press into the message and the tag
 * becomes "typed by you" — a sentence a person rewrote is no longer the
 * machine's.
 */
export const ModelDraft: Story = {
  args: { orgId: "o-1", target: TARGET, onClose: () => {} },
  render: story({
    subject: "Could you introduce me to Philipp Königs?",
    body:
      "Hi Sofia,\n\nYou and Philipp have been in touch (developing, last around " +
      "20 August). I would like to talk to him about the retrofit — would you " +
      "be up for introducing us?\n\nThanks!",
    generated_by: "model",
    ai_generated: true,
    reasoning: [
      {
        kind: "relationship",
        label: "Sofia Meier knows Philipp Königs (developing)",
      },
      { kind: "deal", label: "Retrofit 2026" },
    ],
  }),
};

/**
 * No model configured, so the template wrote it — and says so. The message is
 * still sendable, which is the whole reason this site has a floor and the role
 * reading does not.
 */
export const TemplateDraft: Story = {
  args: { orgId: "o-1", target: TARGET, onClose: () => {} },
  render: story({
    subject: "Could you introduce me to Philipp Königs?",
    body:
      "Hi Sofia,\n\nYou and Philipp Königs have been in touch (developing, last " +
      "around 2026-08-20). Would you be up for introducing us?\n\nIt is about " +
      "Retrofit 2026.\n\nThanks!",
    generated_by: "deterministic",
    ai_generated: false,
    reasoning: [
      {
        kind: "relationship",
        label: "Sofia Meier knows Philipp Königs (developing)",
      },
    ],
  }),
};

/** The colleague has no recorded route, so there is no favour to ask. */
export const Refused: Story = {
  args: { orgId: "o-1", target: TARGET, onClose: () => {} },
  render: story({ code: "not_found", title: "Not Found" }, 404),
};
