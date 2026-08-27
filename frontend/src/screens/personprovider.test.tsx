/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { PersonProviderSection } from "./personprovider";
import { providerCompletedProfile } from "./personprovider.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Buying contact data is the one thing on this page that spends money, and the
// contact nobody has bought for is the case that has to work: a rep who turned
// automatic lookups off reaches the provider ONLY through this panel, so a
// verb they cannot find is a capability they do not have.

type Profile = components["schemas"]["PersonProviderProfile"];

afterEach(() => {
  cleanup();
});

/** A contact nobody has looked up: EVERY bought field empty, not merely the
 *  headline ones. The server has no run to fold values out of, so a fixture
 *  that kept a location or a department would be a payload this state cannot
 *  produce — and would quietly prove the plate on a profile that has data.
 *
 *  `provider` is absent for the same reason: it is filled from the newest run,
 *  so the panel has to name the provider itself to send a request at all. */
function neverRun(): Profile {
  return {
    ...providerCompletedProfile,
    state: "never_run",
    provider: undefined,
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

function mount(profile: Profile, run: () => Response) {
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ person: ["read", "update"] }),
    "POST /people/p-1/enrichment-runs": (body) => {
      posted.push(body);
      return run();
    },
  });
  render(
    <StoryProviders>
      <PersonProviderSection personId="p-1" profile={profile} />
    </StoryProviders>,
  );
  return posted;
}

const queuedRun = () =>
  jsonResponse(
    {
      id: "run-9",
      subject_kind: "person",
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

describe("a contact nobody has bought data for", () => {
  it("offers the lookup as a named plate rather than only a button in the header", async () => {
    mount(neverRun(), queuedRun);

    // The plate names what is missing AND what a lookup costs. Without it the
    // panel is a blank body whose only verb sits in the header corner.
    const title = await screen.findByText(
      "Nothing bought for this contact yet",
    );
    expect(await screen.findByText(/It spends credits/)).toBeDefined();

    // The verb lives INSIDE the plate, which is the whole point: a button that
    // stayed in the header corner would satisfy every text assertion above
    // while leaving the invitation exactly as easy to miss as before.
    const plate = title.closest(".empty-instructional");
    expect(plate).not.toBeNull();
    expect(plate?.querySelector(".empty-action button")).not.toBeNull();
  });

  it("asks the provider named in the contract when the profile carries none", async () => {
    const user = userEvent.setup();
    const posted = mount(neverRun(), queuedRun);

    await user.click(
      await screen.findByRole("button", { name: /Look this contact up/ }),
    );

    // One request, naming a provider. A body without one is refused by the
    // contract, which is the failure a profile-derived name would cause here.
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({ provider: "surfe" });
  });

  it("shows the refusal when the lookup cannot even be queued", async () => {
    const user = userEvent.setup();
    mount(neverRun(), () =>
      jsonResponse(
        {
          title: "No provider is connected",
          status: 404,
          detail: "Connect a data provider before looking a contact up.",
        },
        404,
      ),
    );

    await user.click(
      await screen.findByRole("button", { name: /Look this contact up/ }),
    );

    // The server's own words, not a generic line: a refusal nobody can read is
    // indistinguishable from a click that did nothing.
    expect(
      await screen.findByText(
        "Connect a data provider before looking a contact up.",
      ),
    ).toBeDefined();
  });
});

describe("a contact whose data was already bought", () => {
  it("keeps the values and offers a re-run from the header", async () => {
    mount(providerCompletedProfile, queuedRun);

    // The purchased values themselves, not merely the absence of the plate: a
    // panel that rendered nothing at all would pass a queryByText assertion
    // and hide everything this section exists to show.
    expect(
      await screen.findByText(providerCompletedProfile.emails[0].value),
    ).toBeDefined();
    // The plate belongs to the empty case only — a panel with values that also
    // said "nothing bought yet" would contradict what is under it.
    expect(
      screen.queryByText("Nothing bought for this contact yet"),
    ).toBeNull();
    expect(
      await screen.findByRole("button", { name: /Look this contact up/ }),
    ).toBeDefined();
  });

  // A merge cancels the losing side's queued run and relinks the claims both
  // sides paid for, so the survivor can read never_run while holding
  // purchases. Keying the plate on the state alone would cover them and
  // invite a second lookup for data already on file.
  it("shows purchases a cancelled run left behind instead of claiming nothing was bought", async () => {
    mount({ ...providerCompletedProfile, state: "never_run" }, queuedRun);

    expect(
      await screen.findByText(providerCompletedProfile.emails[0].value),
    ).toBeDefined();
    expect(
      screen.queryByText("Nothing bought for this contact yet"),
    ).toBeNull();
  });
});
