/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import {
  keepPolling,
  PersonProviderSection,
  POLL_LIMIT,
} from "./personprovider";
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
    //
    // The category list is present and EMPTY: this mount serves no connections,
    // so the catalog settles with nothing free, and the button sends what it
    // knows rather than omitting the field. An omitted field asks the server
    // for the connection's whole permitted selection, priced categories
    // included; an empty one is refused by minItems, which is a refusal the
    // reader can see instead of a purchase they cannot.
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({ provider: "surfe", categories: [] });
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

    // The PROVIDER is what this case is about. The empty category list rides
    // along because this mount serves no connections, so the catalog settles
    // with nothing free — asserted rather than elided, since a body that
    // silently dropped the field would be asking for the whole priced
    // selection.
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({ provider: "acmedata", categories: [] });
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

    const line = await screen.findByText(/Asked for, none found/);
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
  // Surfe's real shape: the mobile lookup is only issued when a work email
  // comes back, so its price is the PAIR and `requires` names the other half.
  // A fixture pricing it alone would model a provider that does not exist and
  // would let the button send the request the server now refuses.
  const catalog = [
    { category: "linkedin_profile", free: true, cost: {} },
    { category: "professional_email", free: false, cost: { email: 1 } },
    {
      category: "mobile",
      free: false,
      cost: { mobile: 1, email: 1 },
      requires: "professional_email",
    },
  ];

  function mountWithCatalog(
    profile: Profile,
    run: () => Response,
    // Which categories the admin has switched on. The default is all three,
    // which is what almost every case wants; a case about the SELECTION says
    // so here rather than building a second stub beside this one.
    categories: Record<string, boolean> = {
      linkedin_profile: true,
      professional_email: true,
      mobile: true,
    },
  ) {
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
              configuration: { categories },
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

  // The free button must never send an OMITTED category list.
  //
  // An omitted list asks the server for the connection's whole permitted
  // selection, priced categories included (runcategories.go) — so a press
  // before the catalog arrives spent credits under a label that says free, and
  // the wider an admin sets the selection the more it cost. The catalog is what
  // says which categories are free, so until it lands the button has nothing
  // safe to send.
  it("withholds the free lookup until the catalog says what free means", async () => {
    const posted: unknown[] = [];
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      // A request that never settles, which IS the loading window. Omitting the
      // route instead would resolve to the stub's fallback and settle the
      // query — the button would then be enabled for the honest reason that
      // the catalog answered, and this case would pass while testing nothing.
      "GET /provider-connections": () =>
        new Response(new ReadableStream({ start() {} }), {
          headers: { "Content-Type": "application/json" },
        }),
      "POST /people/p-1/enrichment-runs": (body) => {
        posted.push(body);
        return queuedRun();
      },
    });
    render(
      <StoryProviders>
        <PersonProviderSection
          personId="p-1"
          profiles={[{ ...neverRun(), state: "completed" }]}
        />
      </StoryProviders>,
    );

    const button = await screen.findByRole("button", { name: /Check again/ });
    expect(button).toHaveProperty("disabled", true);
    await userEvent.setup().click(button);
    expect(posted).toEqual([]);
  });

  it("offers each priced detail with its price on the button", async () => {
    mountWithCatalog({ ...neverRun(), state: "completed" }, queuedRun);

    // The price is ON the button: the decision is what to spend, and a button
    // that hid its cost would ask somebody to agree to a number they cannot see.
    expect(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    ).toBeDefined();
    // The mobile button names BOTH halves and the price of both. The provider
    // will not look for a number without an email to anchor it, so a button
    // offering "mobile, 1 credit" would promise a purchase that cannot happen
    // and understate what it spends.
    //
    // Joined by "and", not a comma: a comma read as a list of things the press
    // might pick from, and a rep who took it for the work-email button bought
    // a mobile number he had not asked for.
    expect(
      await screen.findByRole("button", {
        name: /Buy work email and mobile number · 2 credits/,
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

    // Anchored on the price, because the mobile button names the work email
    // too — it cannot be bought without one — and a loose /Buy work email/
    // matches both.
    await user.click(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    );

    // One category, named. A press that sent the connection's whole selection
    // would spend on the mobile too, which nobody asked for.
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({
      provider: "surfe",
      categories: ["professional_email"],
    });
  });

  it("buys the email with the mobile, because the provider will not look for one without it", async () => {
    const user = userEvent.setup();
    const posted = mountWithCatalog(
      { ...neverRun(), state: "completed" },
      queuedRun,
    );

    await user.click(
      await screen.findByRole("button", {
        name: /Buy work email and mobile number · 2 credits/,
      }),
    );

    // BOTH categories, and the email first. Surfe always sends
    // skipMobileEnrichmentIfNoEmailFound, so a request naming the mobile alone
    // makes it hunt for an email nobody bought, fail, and skip the number —
    // returning a run that COMPLETED with nothing in it. The server refuses
    // that request now; this is what stops the button making it.
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({
      provider: "surfe",
      categories: ["professional_email", "mobile"],
    });
  });

  it("offers the mobile with the email already held, and says the email is re-bought", async () => {
    mountWithCatalog(
      {
        ...neverRun(),
        state: "completed",
        // The 819-contact case in a real installation: a work email bought
        // earlier, no number yet.
        emails: [
          {
            value: "dana.buyer@surfe.example",
            email_type: "professional",
            email_type_source: "provider",
            validation_status: "valid",
          },
        ],
      },
      queuedRun,
    );

    // Still offered. Filtering on the button's own category alone was right;
    // filtering on the whole press would have removed the only way to buy a
    // number for somebody whose email is already on record.
    const button = await screen.findByRole("button", {
      name: /Buy mobile number · 2 credits/,
    });
    expect(button).toBeDefined();

    // And the second credit is named. Surfe charges per pool that returns
    // anything and cannot be told to skip an address we already hold, so this
    // press pays for that email twice — a fact the reader has to see BEFORE
    // pressing, not discover on the spend history.
    expect(
      await screen.findByText(/includes the work email again/),
    ).toBeDefined();

    // The button does not offer to buy the email: the reader can see they
    // have it, and naming it there would read as a second purchase of a
    // detail already on screen.
    expect(
      screen.queryByRole("button", { name: /Buy work email and mobile/ }),
    ).toBeNull();
  });

  it("still offers the work email when only a personal address came back", async () => {
    mountWithCatalog(
      {
        ...neverRun(),
        state: "completed",
        // A personal address and nothing else. The two are separate purchases
        // from separate pools, so this contact still has no work email.
        emails: [
          {
            value: "dana@privatemail.example",
            email_type: "personal",
            email_type_source: "provider",
            validation_status: "valid",
          },
        ],
      },
      queuedRun,
    );

    // Reading "any address at all" hid each email offer behind the other: a
    // contact whose personal address came back was never offered a work
    // email, and the reverse.
    expect(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    ).toBeDefined();

    // And the mobile press does not claim to re-buy a work email nobody
    // holds — the note names what is actually paid for twice.
    expect(
      await screen.findByRole("button", {
        name: /Buy work email and mobile number · 2 credits/,
      }),
    ).toBeDefined();
    expect(screen.queryByText(/includes the work email again/)).toBeNull();
  });

  it("offers both email purchases when the provider classified neither", async () => {
    mountWithCatalog(
      {
        ...neverRun(),
        state: "completed",
        // Surfe can omit emailType, and the platform only labels it from the
        // frozen cascade where it can. An address of unknown type is one of
        // the two purchases, and guessing which would either hide an offer
        // the reader can still use or claim a re-buy that never happens.
        emails: [
          {
            value: "dana@unknown.example",
            email_type: null,
            email_type_source: null,
            validation_status: null,
          },
        ],
      },
      queuedRun,
    );

    expect(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    ).toBeDefined();
    // No re-buy claim either: nothing here is known to be the work email, so
    // saying the press pays for one twice would be a guess presented as a fact.
    expect(screen.queryByText(/includes the work email again/)).toBeNull();
  });

  it("says nothing about re-buying when neither half is held", async () => {
    mountWithCatalog({ ...neverRun(), state: "completed" }, queuedRun);

    // The note is the exception, not the furniture. A line about re-buying on
    // every button would train the reader to skip it, which is exactly when
    // it matters.
    await screen.findByRole("button", {
      name: /Buy work email and mobile number · 2 credits/,
    });
    expect(screen.queryByText(/includes the work email again/)).toBeNull();
  });

  it("offers nothing once both halves of a press are held", async () => {
    mountWithCatalog(
      {
        ...neverRun(),
        state: "completed",
        emails: [
          {
            value: "dana.buyer@surfe.example",
            email_type: "professional",
            email_type_source: "provider",
            validation_status: "valid",
          },
        ],
        mobile_phones: [{ value: "+491701234567", confidence: 0.82 }],
      },
      queuedRun,
    );

    await screen.findByText(/dana\.buyer@surfe\.example/);
    // Nothing left to look for, so no offer — and in particular not a
    // two-credit button that would buy both again.
    expect(screen.queryByRole("button", { name: /^Buy / })).toBeNull();
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

// The watch arms off the RUN, never off the section's state.
//
// The section answers "what should this reader know about enrichment here", and
// the CONNECTION's condition outranks the run history in that answer — so on a
// connection that last failed, the section reads provider_error while a run the
// reader just started is genuinely in flight. Arming off the section left the
// watch asleep through exactly that lookup: it completed, the page never
// re-read, and for the half-minute it took the button looked dead. That is the
// state a reader is MOST likely to be pressing from, because the message beside
// it tells them to.
describe("watching a run that is still moving", () => {
  const inFlight = (): components["schemas"]["ProviderRun"] => ({
    ...completedProviderRun,
    id: "run-live",
    state: "in_progress",
    applied: false,
  });

  // A run the platform stops advancing leaves its flags exactly as they are,
  // and every ordinary way out of `stillMoving` is a state the platform writes.
  // Without a ceiling an open tab asks after it every 2.5 seconds until it is
  // closed — so the page gives up, which is a thing a reader can see.
  it("stops asking after a run the platform never finishes", () => {
    const stuck = {
      ...completedProviderRun,
      id: "run-stuck",
      applied: false,
    };
    // The rule is still true about the run itself: this is a ceiling on asking,
    // not a claim that the run finished.
    expect(keepPolling(stuck, 0)).toBe(true);
    expect(keepPolling(stuck, POLL_LIMIT - 1)).toBe(true);
    expect(keepPolling(stuck, POLL_LIMIT)).toBe(false);
  });

  // The half a reader can see. The poll below is what makes the page correct;
  // this is what stops it looking broken while it works.
  it("says a lookup is happening, and does not report the old failure over it", async () => {
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      "GET /people/p-1/enrichment-runs/run-live": () =>
        jsonResponse(inFlight()),
    });
    render(
      <StoryProviders>
        <PersonProviderSection
          personId="p-1"
          profiles={[
            { ...neverRun(), state: "provider_error", latest_run: inFlight() },
          ]}
        />
      </StoryProviders>,
    );

    expect(await screen.findByText(/Asking Surfe/)).toBeDefined();
    // The connection's last failure is real and belongs on the page at rest.
    // Over a lookup the reader is watching it is a stale fact dressed as a
    // live one, which is what sent Lars looking for a broken button.
    expect(screen.queryByText(/last call to the provider failed/)).toBeNull();
    expect(await screen.findByText("Looking them up…")).toBeDefined();
  });

  // Two phases, and the reader is told which. Once the provider has answered,
  // saying "asking Surfe" reports work nobody is doing — the vendor is done and
  // the values are being folded onto the record.
  it("says the answer is landing once the provider has finished", async () => {
    const unapplied = {
      ...completedProviderRun,
      id: "run-landing",
      applied: false,
    };
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      "GET /people/p-1/enrichment-runs/run-landing": () =>
        jsonResponse(unapplied),
    });
    render(
      <StoryProviders>
        <PersonProviderSection
          personId="p-1"
          profiles={[
            { ...neverRun(), state: "completed", latest_run: unapplied },
          ]}
        />
      </StoryProviders>,
    );

    expect(
      await screen.findByText("Answer received. Putting it on the record."),
    ).toBeDefined();
    expect(screen.queryByText(/Asking Surfe/)).toBeNull();
  });

  // The money case. The server's duplicate-spend fence covers the LIVE run
  // states only, so a completed-but-unapplied run is buyable again — and the
  // claim has not landed yet, so nothing else hides the button either. Two
  // presses, two charges, for one detail.
  it("offers no purchase button while a run's values are still landing", async () => {
    const unapplied = {
      ...completedProviderRun,
      id: "run-landing-2",
      applied: false,
    };
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      "GET /people/p-1/enrichment-runs/run-landing-2": () =>
        jsonResponse(unapplied),
    });
    render(
      <StoryProviders>
        <PersonProviderSection
          personId="p-1"
          profiles={[
            { ...neverRun(), state: "completed", latest_run: unapplied },
          ]}
        />
      </StoryProviders>,
    );

    await screen.findByText("Answer received. Putting it on the record.");
    expect(screen.queryByRole("button", { name: /Buy / })).toBeNull();
    expect(screen.queryByRole("button", { name: /Check again/ })).toBeNull();
  });

  it("polls a live run even while the connection reads as failed", async () => {
    let polls = 0;
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      "GET /people/p-1/enrichment-runs/run-live": () => {
        polls += 1;
        return jsonResponse(inFlight());
      },
    });
    render(
      <StoryProviders>
        <PersonProviderSection
          personId="p-1"
          profiles={[
            // The section is impaired; the run underneath it is not.
            { ...neverRun(), state: "provider_error", latest_run: inFlight() },
          ]}
        />
      </StoryProviders>,
    );

    await expect.poll(() => polls).toBeGreaterThan(0);
  });

  // A run whose values have not been folded onto the record yet is not done
  // from this page's point of view: the apply commits AFTER the run completes,
  // so a watch that stopped at `completed` refreshed one step before the thing
  // the reader is waiting for existed.
  it("keeps watching a completed run until its values are applied", async () => {
    let polls = 0;
    const unapplied = {
      ...completedProviderRun,
      id: "run-unapplied",
      applied: false,
    };
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      "GET /people/p-1/enrichment-runs/run-unapplied": () => {
        polls += 1;
        return jsonResponse(unapplied);
      },
    });
    render(
      <StoryProviders>
        <PersonProviderSection
          personId="p-1"
          profiles={[
            { ...neverRun(), state: "completed", latest_run: unapplied },
          ]}
        />
      </StoryProviders>,
    );

    await expect.poll(() => polls).toBeGreaterThan(0);
  });

  // And it stops. A watch with no end condition is a request every 2.5 seconds
  // for as long as the tab is open.
  it("does not watch a run whose values are already on the record", async () => {
    let polls = 0;
    installFetchStub({
      "GET /me": meRoute({ person: ["read", "update"] }),
      "GET /people/p-1/enrichment-runs/run-1": () => {
        polls += 1;
        return jsonResponse(completedProviderRun);
      },
    });
    render(
      <StoryProviders>
        <PersonProviderSection
          personId="p-1"
          profiles={[providerCompletedProfile]}
        />
      </StoryProviders>,
    );

    await screen.findByRole("button", { name: /Check again/ });
    expect(polls).toBe(0);
  });
});
