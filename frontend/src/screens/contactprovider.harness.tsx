import { render } from "@testing-library/react";
import type { components } from "../api/schema";
import { ContactProviderSection } from "./contactprovider";
import {
  completedProviderRun,
  providerCompletedProfile,
} from "./contactprovider.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The fixtures and the mount every case of this panel starts from. They live
// beside the cases rather than inside one file because two test files ask
// about the same screen — what it SHOWS, and what its button CLAIMS — and a
// second copy of a profile fixture is how the two would start disagreeing
// about what a contact nobody looked up looks like.

export type Profile = components["schemas"]["ContactProviderProfile"];

/** A contact nobody has looked up: EVERY bought field empty, not merely the
 *  headline ones. The server has no run to fold values out of, so a fixture
 *  that kept a location or a department would be a payload this state cannot
 *  produce — and would quietly prove the plate on a profile that has data.
 *
 *  `provider` is set: a section belongs to one named vendor whether or not a
 *  run exists, which is what lets the reader tell who they are about to pay. */
export function neverRun(): Profile {
  return {
    ...providerCompletedProfile,
    state: "never_run",
    provider: "surfe",
    retrieved_at: null,
    emails: [],
    mobile_phones: [],
    linkedin_url: null,
    current_employment: undefined,
    job_history: [],
    location: null,
    departments: [],
    seniorities: [],
    latest_run: undefined,
    contributing_runs: undefined,
    categories_not_requested: [],
  };
}

// A contact whose lookup COMPLETED and retained nothing free to show — the
// shape the priced-category cases are about. The section state and the
// run are ONE fact: a profile reading completed with no run on the record is
// one no server sends, and a fixture that fabricates it lets a screen reading
// the state stand in for a screen reading the history.
export function completedKeepingNothing(): Profile {
  return {
    ...neverRun(),
    state: "completed",
    latest_run: completedProviderRun,
  };
}

export function mount(profile: Profile, run: () => Response) {
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }),
    "POST /contacts/p-1/enrichment-runs": (body) => {
      posted.push(body);
      return run();
    },
  });
  render(
    <StoryProviders>
      <ContactProviderSection contactId="p-1" profiles={[profile]} />
    </StoryProviders>,
  );
  return posted;
}

export const queuedRun = () =>
  jsonResponse(
    {
      id: "run-9",
      subject_kind: "contact",
      provider: "surfe",
      trigger: "manual",
      state: "queued",
      claims_unwritten: false,
      requested_categories: ["professional_email"],
      connection_version: 1,
      created_at: "2026-08-20T09:00:00Z",
      updated_at: "2026-08-20T09:00:00Z",
      completed_at: null,
    },
    202,
  );
