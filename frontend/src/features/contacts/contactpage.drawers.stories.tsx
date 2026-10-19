// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { ContactMeetingBrief } from "../brief/meetingbrief";
import { ProviderCard } from "../capture/integrations-provider";
import { ContactResearchDrawer } from "./contactdrawers";
import {
  completedProviderRun,
  providerCompletedProfile,
} from "./contactprovider.fixtures";
import "./contact360.css";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";

// The contact page's research drawer and meeting brief, from the page gallery
// (contactpage.stories.tsx). Each story stubs the provider reads it needs.

const meta: Meta = {
  title: "Records/Contact 360/Page drawers",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// No provider is registered. The profile is PRESENT as "not_connected": absent
// would mean the reader lacks the grant, which is a different fact.
const providerNotConnected: components["schemas"]["ContactProviderProfile"] = {
  provider: "surfe",
  state: "not_connected",
  categories_not_requested: [],
  emails: [],
  mobile_phones: [],
  job_history: [],
  departments: [],
  seniorities: [],
};

export const ResearchDrawer: Story = {
  render: () => {
    installFetchStub({
      "POST /contacts/p-1/research": () =>
        jsonResponse({
          contact_id: "p-1",
          state: "not_connected",
          generated_at: "2026-08-13T09:00:00Z",
          claims: [],
        }),
    });
    return (
      <StoryProviders>
        <ContactResearchDrawer
          contactId="p-1"
          contactName="Dana Buyer"
          providerProfiles={[providerNotConnected]}
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
};

// A run that landed with values on every branch ProviderValues renders
// (contactprovider.tsx). `providerNotConnected` above leaves every array
// empty. Without this fixture, none of emails/mobiles/employment/job history/
// location/departments/seniorities, nor the EnrichNow button, ever render.

export const ResearchDrawerProviderCompleted: Story = {
  render: () => {
    installFetchStub({
      "POST /contacts/p-1/research": () =>
        jsonResponse({
          contact_id: "p-1",
          state: "ready",
          generated_at: "2026-08-13T09:00:00Z",
          claims: [],
        }),
    });
    return (
      <StoryProviders>
        <ContactResearchDrawer
          contactId="p-1"
          contactName="Dana Buyer"
          providerProfiles={[providerCompletedProfile]}
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
};

// A run still moving: RunWatch polls `GET .../enrichment-runs/{run_id}`
// while `isRunning` holds, and the badge reads "In progress" rather than any
// terminal word: a state `providerNotConnected` and `providerCompletedProfile`
// above never reach.
const inProgressRun: components["schemas"]["ProviderRun"] = {
  ...completedProviderRun,
  id: "run-2",
  state: "in_progress",
  // Overridden, not inherited: the base is a run that landed, and the contract
  // says applied is always false for a run that never completed. Spreading it
  // would have this story claim values reached the record while the run was
  // still in flight.
  applied: false,
  completed_at: null,
  submitted_at: "2026-08-13T09:00:00Z",
  requested_categories: ["email"],
};

const providerRunning: components["schemas"]["ContactProviderProfile"] = {
  state: "in_progress",
  provider: "surfe",
  retrieved_at: null,
  // This run asked for email only, so mobile is "not requested" rather than
  // "requested but not yet back". The run has nothing to show for either
  // while it is still moving.
  categories_not_requested: ["mobile"],
  emails: [],
  mobile_phones: [],
  job_history: [],
  departments: [],
  seniorities: [],
  latest_run: inProgressRun,
};

export const ResearchDrawerProviderRunning: Story = {
  render: () => {
    installFetchStub({
      "POST /contacts/p-1/research": () =>
        jsonResponse({
          contact_id: "p-1",
          state: "not_connected",
          generated_at: "2026-08-13T09:00:00Z",
          claims: [],
        }),
      "GET /contacts/p-1/enrichment-runs/run-2": () =>
        jsonResponse(inProgressRun),
    });
    return (
      <StoryProviders>
        <ContactResearchDrawer
          contactId="p-1"
          contactName="Dana Buyer"
          providerProfiles={[providerRunning]}
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
};

// A run the provider refused: paid nothing (no run reached the vendor's
// meter) and returned nothing. So the badge carries the danger tone
// (provider-status.ts's PROFILE_TONE), not the neutral or success one every
// other provider fixture here shows.
const failedRun: components["schemas"]["ProviderRun"] = {
  ...completedProviderRun,
  id: "run-3",
  state: "failed",
  // Same reason as the in-flight run above: nothing was applied, because
  // nothing was bought.
  applied: false,
  completed_at: "2026-08-13T09:05:00Z",
  safe_status_code: "provider_unavailable",
};

const providerError: components["schemas"]["ContactProviderProfile"] = {
  state: "provider_error",
  provider: "surfe",
  retrieved_at: null,
  safe_status_code: "provider_unavailable",
  categories_not_requested: [],
  emails: [],
  mobile_phones: [],
  job_history: [],
  departments: [],
  seniorities: [],
  latest_run: failedRun,
};

export const ResearchDrawerProviderError: Story = {
  render: () => {
    installFetchStub({
      "POST /contacts/p-1/research": () =>
        jsonResponse({
          contact_id: "p-1",
          state: "not_connected",
          generated_at: "2026-08-13T09:00:00Z",
          claims: [],
        }),
    });
    return (
      <StoryProviders>
        <ContactResearchDrawer
          contactId="p-1"
          contactName="Dana Buyer"
          providerProfiles={[providerError]}
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
};

export const MeetingBrief: Story = {
  render: () => {
    installFetchStub({
      "GET /activities/a-2/meeting-brief": () =>
        jsonResponse({
          activity_id: "a-2",
          generated_at: "2026-08-13T09:00:00Z",
          generated_by: "deterministic",
          sections: [
            {
              kind: "goal",
              sentences: [
                {
                  text: "Confirm the retrofit timeline and lock the depot offline window.",
                  evidence: [{ entity_type: "activity", entity_id: "a-1" }],
                },
              ],
            },
            {
              kind: "commitments",
              sentences: [
                {
                  text: "You owe Dana the updated retrofit quote.",
                  evidence: [{ entity_type: "activity", entity_id: "a-1" }],
                },
              ],
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <ContactMeetingBrief activityId="a-2" open onClose={() => {}} />
      </StoryProviders>
    );
  },
};

// --- Provider section: the not-configured state -----------------------------
// No provider key: the server answers 501 code:not_implemented.
export const ProviderNotConfigured: Story = {
  render: () => {
    installFetchStub({
      // ProviderCard reads the session to decide whether the surface is
      // read-only. So the probe needs a route, even in a story about the
      // server answering 501.
      "GET /me": meRoute({ integrations: ["read"] }),
      "GET /provider-connections": () =>
        jsonResponse({ code: "not_implemented" }, 501),
    });
    return (
      <StoryProviders>
        <ProviderCard />
      </StoryProviders>
    );
  },
};
