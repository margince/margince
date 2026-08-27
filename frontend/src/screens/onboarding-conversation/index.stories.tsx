// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../../app/mefixture";
import { configuredAiProfile } from "../onboarding.stories.fixtures";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../story-utils";
import { OnboardingConversationScreen } from "./index";

// The conversation SHELL, as opposed to the acts inside it (workbench.stories,
// entries.stories, confirm-card.stories). What this file documents is the one
// thing only the shell does: it reads the server's truth on mount — the wizard
// row, the company profile, the voice probe, the persisted read — and decides
// from those four answers where the reader belongs.
//
// That decision is a REDIRECT when the journey is already finished, and it is
// one of the app's only two automatic navigators. It replaces the current
// history entry rather than pushing one, because an address the product answers
// by sending the reader elsewhere must not be somewhere Back can land: it would
// redirect again, and the reader could not get out with the one key that exists
// for getting out of things.
//
// Every story routes the session probe explicitly. An unrouted one answers the
// stub's list shape, which reads as a MALFORMED session rather than an absent
// one, and the shell then renders a refusal none of these stories is named for.

const session = () => jsonResponse(meFixture({ allow: {} }));

/** The wizard row as the server stores it, at a named step. */
function wizardAt(step: string, path: "creator" | "member" = "creator") {
  return {
    path,
    step,
    source_mode: "website",
    website_url: "https://gradion.com",
    site_read_id: null,
    company_draft: {},
    selected_fact_keys: [],
    voice_skipped: false,
    connect_skipped: false,
    version: 1,
    completed_at: null,
    created_at: "2026-08-20T08:00:00Z",
    updated_at: "2026-08-20T09:00:00Z",
  };
}

const COMPANY = {
  organization_id: "018f3a1b-0000-7000-8000-0000000000a1",
  display_name: "Gradion",
  website: "gradion.com",
  offer_summary: "Revenue software for manufacturers",
  icp: "Mid-market manufacturers",
};

function shell(routes: RouteMap) {
  installFetchStub({
    "GET /me": session,
    "GET /ai/profile": () => jsonResponse(configuredAiProfile),
    "GET /company/context/capabilities": () =>
      jsonResponse({
        onboarding_enabled: true,
        read_enabled: true,
        rollout: "ga",
      }),
    "GET /voice-profiles": () => jsonResponse({ data: [], page: {} }),
    "GET /connectors": () => jsonResponse({ data: [], page: {} }),
    ...routes,
  });
  return (
    <StoryProviders>
      <OnboardingConversationScreen />
    </StoryProviders>
  );
}

const meta: Meta<typeof OnboardingConversationScreen> = {
  title: "Onboarding/Conversation/Shell",
  component: OnboardingConversationScreen,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof OnboardingConversationScreen>;

export const AFirstRun: Story = {
  // No wizard row and no company: the shell has nothing to restore, so it opens
  // the conversation at its beginning rather than resuming anybody.
  render: () =>
    shell({
      "GET /onboarding/state": () => jsonResponse({ detail: "none" }, 404),
      "GET /company": () => jsonResponse({ detail: "no company yet" }, 404),
    }),
};

export const ResumingMidJourney: Story = {
  // A wizard row parked on `voice` with the company already described: the
  // shell restores to the act the reader left rather than starting over, which
  // is the whole reason it reads the server before rendering anything.
  render: () =>
    shell({
      "GET /onboarding/state": () => jsonResponse(wizardAt("voice")),
      "GET /company": () => jsonResponse(COMPANY),
    }),
};

export const StillReadingTheServer: Story = {
  // The restore gate, which is what the reader sees while those four answers
  // are in flight. It is a state of its own and not a blank frame: the shell
  // may not decide where anybody belongs until every lookup has SETTLED, or a
  // transient error would send an existing member down the creator flow.
  render: () =>
    shell({
      "GET /onboarding/state": () => new Promise<Response>(() => {}),
      "GET /company": () => new Promise<Response>(() => {}),
    }),
};

export const TheServerRefusedTheRestore: Story = {
  // A failed lookup is offered as a retry rather than swallowed: the shell
  // cannot route on an answer it does not have, and guessing is what would put
  // a returning member through the creator flow.
  render: () =>
    shell({
      "GET /onboarding/state": () =>
        jsonResponse({ title: "Server error" }, 500),
      "GET /company": () => jsonResponse({ title: "Server error" }, 500),
    }),
};
