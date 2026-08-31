/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { PersonProviderSection } from "./personprovider";
import {
  completedProviderRun,
  providerCompletedProfile,
} from "./personprovider.fixtures";
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
 *  `provider` is set: a section belongs to one named vendor whether or not a
 *  run exists, which is what lets the reader tell who they are about to pay. */
function neverRun(): Profile {
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
      <PersonProviderSection personId="p-1" profiles={[profile]} />
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
    expect(await screen.findByText(/It spends Surfe credits/)).toBeDefined();

    // The verb lives INSIDE the plate, which is the whole point: a button that
    // stayed in the header corner would satisfy every text assertion above
    // while leaving the invitation exactly as easy to miss as before.
    const plate = title.closest(".empty-instructional");
    expect(plate).not.toBeNull();
    expect(plate?.querySelector(".empty-action button")).not.toBeNull();
  });

  it("spends with the provider whose section the button sits in", async () => {
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
    // "Check again", not "look this contact up": this contact HAS been looked
    // up, and the button offers the same free details a second time in case a
    // job changed. The first-lookup wording on a record already showing a
    // lookup reads as the control that fetches the email — the one thing this
    // button never does, because that costs credits and has its own.
    expect(
      await screen.findByRole("button", { name: /Check again/ }),
    ).toBeDefined();
    expect(
      screen.queryByRole("button", { name: /Look this contact up/ }),
    ).toBeNull();
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

describe("two providers connected", () => {
  it("names each section and keeps one provider's values out of the other", async () => {
    const bought = { ...providerCompletedProfile, provider: "surfe" as const };
    const unasked = { ...neverRun(), provider: "acmedata" as const };
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      "GET /people/p-1/enrichment-runs/run-1": () =>
        jsonResponse({ ...completedProviderRun, state: "completed" }),
    });
    render(
      <StoryProviders>
        <PersonProviderSection personId="p-1" profiles={[bought, unasked]} />
      </StoryProviders>,
    );

    // Each section says WHOSE it is. Without the name the reader cannot tell
    // who sold them the address, nor who the button would spend with.
    expect(await screen.findByText("Surfe")).toBeDefined();
    expect(await screen.findByText("acmedata")).toBeDefined();

    // The purchase belongs to the section of the provider that made it. A
    // fold that mixed them would show this address under both headings.
    const surfeSection = screen.getByText("Surfe").closest(".panel");
    expect(surfeSection?.textContent?.includes(bought.emails[0].value)).toBe(
      true,
    );
    const otherSection = screen.getByText("acmedata").closest(".panel");
    expect(otherSection?.textContent?.includes(bought.emails[0].value)).toBe(
      false,
    );
  });

  it("offers a lookup per provider, so the reader chooses who to spend with", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      "POST /people/p-1/enrichment-runs": (body) => {
        posted.push(body);
        return queuedRun();
      },
    });
    render(
      <StoryProviders>
        <PersonProviderSection
          personId="p-1"
          profiles={[
            { ...neverRun(), provider: "surfe" as const },
            { ...neverRun(), provider: "acmedata" as const },
          ]}
        />
      </StoryProviders>,
    );

    // The SECOND section's button spends with the second provider. One shared
    // button, or a body that named the wrong provider, would buy from whoever
    // happened to be first.
    const buttons = await screen.findAllByRole("button", {
      name: /Look this contact up/,
    });
    expect(buttons.length).toBe(2);
    await user.click(buttons[1]);

    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({ provider: "acmedata" });
  });
});

// A lookup spends the installation's credits on a NAMED contact, and a run
// charged to the wrong one succeeds — so nothing on screen reports it. Which
// contact the request is about therefore travels with the click, as a mutation
// variable, rather than being read out of whichever render armed the mutation.
describe("which contact a lookup is charged to", () => {
  it("posts for the contact the panel is showing, not the one it opened on", async () => {
    const user = userEvent.setup();
    const paths: string[] = [];
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      "POST /people/p-1/enrichment-runs": () => {
        paths.push("p-1");
        return queuedRun();
      },
      "POST /people/p-2/enrichment-runs": () => {
        paths.push("p-2");
        return queuedRun();
      },
    });
    const { rerender } = render(
      <StoryProviders>
        <PersonProviderSection personId="p-1" profiles={[neverRun()]} />
      </StoryProviders>,
    );
    await screen.findByRole("button", { name: /Look this contact up/ });

    // The panel stays mounted across the change of subject: the record page
    // keys its subtree by contact today, and this is the case that breaks the
    // moment it stops.
    rerender(
      <StoryProviders>
        <PersonProviderSection personId="p-2" profiles={[neverRun()]} />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: /Look this contact up/ }),
    );

    await expect.poll(() => paths).toEqual(["p-2"]);
  });
});

describe("a lookup that came back mostly empty", () => {
  // The case that prompted this: Surfe was asked for six categories and
  // returned one — an employment whose job title was blank, leaving only a
  // company the record already knew. The section showed a green "Found" badge
  // over a single field, and the reader could not tell whether their lookup
  // had done anything at all.
  function mostlyEmpty(): Profile {
    return {
      ...neverRun(),
      provider: "surfe" as const,
      state: "completed",
      retrieved_at: "2026-08-27T08:36:52Z",
      current_employment: { company_name: "e-Kugellager" },
      categories_asked: [
        "professional_email",
        "linkedin_profile",
        "current_employment",
        "job_history",
        "personal_email",
      ],
      categories_without_answer: [
        "professional_email",
        "linkedin_profile",
        "job_history",
        "personal_email",
      ],
      latest_run: {
        ...completedProviderRun,
        requested_categories: [
          "professional_email",
          "mobile",
          "linkedin_profile",
          "current_employment",
          "job_history",
          "personal_email",
        ],
      },
    };
  }

  it("says how much of what was asked for actually came back", async () => {
    mount(mostlyEmpty(), queuedRun);

    // The count is the answer to "did my lookup do anything": six asked, one
    // returned. Without it a green badge over one field reads as a success.
    expect(
      await screen.findByText(/asked for 5 details, got 1 back/),
    ).toBeDefined();
  });

  it("names the categories the provider had nothing for, in words a rep knows", async () => {
    mount(mostlyEmpty(), queuedRun);

    const line = await screen.findByText(/Asked for and not found/);
    // The provider's vocabulary is a set of keys — `professional_email`,
    // `linkedin_profile`. Printed raw they are not words anybody uses.
    expect(line.textContent).toContain("work email");
    // Surfe skips the mobile lookup when it found no email, so it was
    // never asked and must not be listed as something they lacked.
    expect(line.textContent).not.toContain("mobile number");
    expect(line.textContent).toContain("LinkedIn profile");
    expect(line.textContent).not.toContain("professional_email");

    // And NOT the one that was answered.
    expect(line.textContent).not.toContain("current role");
  });

  it("gives only the date when the server did not say what went unanswered", async () => {
    const unknown = { ...mostlyEmpty(), categories_without_answer: undefined };
    mount(unknown, queuedRun);

    // An older backend, or an adapter that never declared which claim answers
    // which category. The date is a fact; a count derived from a missing list
    // would invent "everything came back", which is the exact reading this
    // line exists to prevent.
    expect(await screen.findByText(/Looked up 27 Aug 2026\./)).toBeDefined();
    expect(screen.queryByText(/asked for/)).toBeNull();
  });

  it("keeps the receipt off a section nobody has run", async () => {
    mount(neverRun(), queuedRun);

    // A receipt for a purchase nobody made would be an invention, and the
    // first-run plate already says what the state is.
    expect(screen.queryByText(/asked for/)).toBeNull();
  });
});

describe("the details that cost credits", () => {
  const catalog = [
    { category: "linkedin_profile", free: true, cost: {} },
    { category: "professional_email", free: false, cost: { email: 1 } },
    { category: "mobile", free: false, cost: { mobile: 1 } },
  ];

  function mountWithCatalog(profile: Profile, run: () => Response) {
    const posted: unknown[] = [];
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      "GET /provider-connections": () =>
        jsonResponse({
          data: [
            {
              provider: "surfe",
              status: "connected",
              credential_present: true,
              catalog,
              configuration: {
                categories: {
                  linkedin_profile: true,
                  professional_email: true,
                  mobile: true,
                },
              },
              credits: { pools: {} },
              version: 1,
              created_at: "2026-08-20T09:00:00Z",
              updated_at: "2026-08-20T09:00:00Z",
            },
          ],
        }),
      "POST /people/p-1/enrichment-runs": (body) => {
        posted.push(body);
        return run();
      },
    });
    render(
      <StoryProviders>
        <PersonProviderSection personId="p-1" profiles={[profile]} />
      </StoryProviders>,
    );
    return posted;
  }

  it("offers each priced detail with its price on the button", async () => {
    mountWithCatalog({ ...neverRun(), state: "completed" }, queuedRun);

    // The price is ON the button: the decision is what to spend, and a button
    // that hid its cost would ask somebody to agree to a number they cannot see.
    expect(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    ).toBeDefined();
    expect(
      await screen.findByRole("button", {
        name: /Buy mobile number · 1 credit/,
      }),
    ).toBeDefined();

    // No button for a free category: it arrives without anybody pressing
    // anything, which is the whole point of the split.
    expect(screen.queryByRole("button", { name: /Buy LinkedIn/ })).toBeNull();
  });

  it("buys only the detail whose button was pressed", async () => {
    const user = userEvent.setup();
    const posted = mountWithCatalog(
      { ...neverRun(), state: "completed" },
      queuedRun,
    );

    await user.click(
      await screen.findByRole("button", { name: /Buy work email/ }),
    );

    // One category, named. A press that sent the connection's whole selection
    // would spend on the mobile too, which nobody asked for.
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({
      provider: "surfe",
      categories: ["professional_email"],
    });
  });

  it("names the free categories when the plain lookup button is pressed", async () => {
    const user = userEvent.setup();
    const posted = mountWithCatalog(
      { ...neverRun(), state: "completed" },
      queuedRun,
    );

    // A run that completed, so the button offers a re-check rather than a
    // first lookup. Either wording drives the same press; what this case pins
    // is WHAT it sends.
    await user.click(
      await screen.findByRole("button", { name: /Check again/ }),
    );

    // The FREE set, named. Sending no categories asks for the connection's
    // whole selection, priced ones included — a button that spends without
    // saying so, which is what the split exists to prevent.
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({
      provider: "surfe",
      categories: ["linkedin_profile"],
    });
  });

  it("does not offer to buy what the section already shows", async () => {
    mountWithCatalog(providerCompletedProfile, queuedRun);

    // The completed fixture already carries an address and a mobile. Offering
    // to buy them again would spend credits on what is on screen.
    await screen.findByText(providerCompletedProfile.emails[0].value);
    expect(screen.queryByRole("button", { name: /Buy work email/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Buy mobile/ })).toBeNull();
  });
});
