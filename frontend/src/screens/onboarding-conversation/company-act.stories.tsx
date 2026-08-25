// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { CompanyAct } from "./company-act";
import { initialConversationState } from "./conversation-machine";
import type {
  ConversationPhase,
  ConversationState,
} from "./conversation-types";

// The first act of the conversation: what this installation is FOR. It reads
// the company's own site, says what it learned, and asks a human to confirm it
// before anything else in the product is built on top.
//
// The act is a reducer plus a surface, so a story is a STATE rather than a set
// of props — `phase` is the whole story list. `initialConversationState` is the
// reducer's own starting point, which is why the fixtures build from it instead
// of asserting a shape of their own.
//
// The eyebrow above the panel says where the reader is standing: "Step 3 of 5",
// a position in a fixed rail. Neither half of it is ever grouped, in any
// locale — and the German stories are where that is visible, because a
// grouped "1.204" in a step counter reads as a quantity of steps.

function state(
  phase: ConversationPhase,
  overrides: Partial<ConversationState> = {},
): ConversationState {
  return {
    ...initialConversationState,
    act: "company",
    phase,
    ...overrides,
  };
}

const PROFILE = {
  id: "co-1",
  display_name: "Brandt Automotive",
  website: "https://brandt-automotive.example",
  legal_name: "Brandt Automotive GmbH",
  industry: "Automotive tier-one supply",
};

// The act probes the session for the seat that may confirm, and persists the
// draft. Both are routed: an unrouted session probe reads as a MALFORMED
// session rather than an absent one, and the panel would then draw the
// fail-closed branch in a story named for something else.
function act(
  conversation: ConversationState,
  profile: typeof PROFILE | null = PROFILE,
  locale?: "de",
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ installation_settings: ["read", "update"] }),
      "GET /company-profile": () =>
        profile ? jsonResponse(profile) : jsonResponse({}, 404),
    });
    return (
      <StoryProviders locale={locale}>
        <CompanyAct
          state={conversation}
          dispatch={() => {}}
          profile={null}
          persist={async () => true}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CompanyAct> = {
  title: "Onboarding/Conversation/Company act",
  component: CompanyAct,
};
export default meta;
type Story = StoryObj<typeof CompanyAct>;

/** Before anything has been read: the act says what it is about to do. */
export const Intro: Story = { render: act(state("co.intro")) };

/** The read in flight. The thread carries the progress in words, so the act
 *  itself does not have to narrate it twice. */
export const Reading: Story = { render: act(state("co.reading")) };

/**
 * The read found something it is not sure of and asks. A question here is the
 * act's own, not a form field — the conversation is the interface, and a
 * clarification that appeared as an input would lose the thread it belongs to.
 */
export const Clarifying: Story = {
  render: act(state("co.clarify", { pendingQuestion: null })),
};

/** What was learned, for a human to confirm. The step eyebrow is above it. */
export const Review: Story = {
  render: act(state("co.review", { readCompleted: true })),
};

/** The German act: the same step, the longer copy, and the position pair in the
 *  reader's own notation — ungrouped, because it is a place rather than a
 *  quantity. */
export const ReviewGerman: Story = {
  render: act(state("co.review", { readCompleted: true }), PROFILE, "de"),
};

/**
 * The read found nothing usable and the reader types it instead. Not a failure
 * state: a company with no site is an ordinary customer, and the act has to
 * carry them just as far.
 */
export const Manual: Story = { render: act(state("co.manual"), null) };

/** Confirmed, and waiting to be routed onward. A reducer cannot advance itself
 *  out of a momentary state, so a restored session lands here. */
export const Confirmed: Story = {
  render: act(state("co.confirmed", { readCompleted: true })),
};

/** At 390px the panel is the whole screen and the step eyebrow shares its row
 *  with nothing. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: act(state("co.review", { readCompleted: true })),
};
