/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  ContactProviderSection,
  keepPolling,
  POLL_LIMIT,
} from "./contactprovider";
import {
  completedProviderRun,
  providerCompletedProfile,
} from "./contactprovider.fixtures";
import {
  mount,
  neverRun,
  type Profile,
  queuedRun,
} from "./contactprovider.harness";

afterEach(() => {
  cleanup();
});

describe("a contact nobody has bought data for", () => {
  it("offers the lookup as a named plate rather than only a button in the header", async () => {
    mount(neverRun(), queuedRun);

    // The plate names what is missing AND what a lookup costs. Without it the
    // panel is a blank body whose only verb sits in the header corner.
    const title = await screen.findByText(en["provider.profile.emptyTitle"]);
    expect(await screen.findByText(/spends Surfe credits/)).toBeDefined();

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
      await screen.findByRole("button", { name: /Look up contact/ }),
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
      await screen.findByRole("button", { name: /Look up contact/ }),
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
    expect(screen.queryByText(en["provider.profile.emptyTitle"])).toBeNull();
    // "Check again", not "look this contact up": this contact HAS been looked
    // up, and the button offers the same free details a second time in case a
    // job changed. The first-lookup wording on a record already showing a
    // lookup reads as the control that fetches the email — the one thing this
    // button never does, because that costs credits and has its own.
    expect(
      await screen.findByRole("button", { name: /Check again/ }),
    ).toBeDefined();
    expect(
      screen.queryByRole("button", { name: /Look up contact/ }),
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
    expect(screen.queryByText(en["provider.profile.emptyTitle"])).toBeNull();
  });
});

// The two wordings make a claim about this contact's HISTORY — was the provider
// ever asked about them — and the section's folded state answers a different
// question: the connection's condition ahead of the run. Reading the wording
// off that state is wrong in both directions, and both directions are here,
// because a reader told the opposite of the truth is worst on an impaired
// connection — exactly when somebody is working out why a lookup found nothing.
describe("the lookup button's wording follows the contact's run history", () => {
  it("offers a first lookup over a contact no run ever touched, whatever the connection says", async () => {
    // An impaired connection gives every one of its contacts a state that is
    // not never_run. Nothing was looked up; nothing was bought.
    mount({ ...neverRun(), state: "invalid_credentials" }, queuedRun);

    expect(
      await screen.findByRole("button", { name: /Look up contact/ }),
    ).toBeDefined();
    expect(screen.queryByRole("button", { name: /Check again/ })).toBeNull();
  });

  it("offers a re-check over a contact whose only run was cancelled and kept nothing", async () => {
    // A cancelled latest run folds back to never_run and retains no values, so
    // the state and the values agree on "first lookup" and both are wrong: the
    // run is on the record.
    mount(
      {
        ...neverRun(),
        latest_run: { ...completedProviderRun, state: "cancelled" },
      },
      queuedRun,
    );

    expect(
      await screen.findByRole("button", { name: /Check again/ }),
    ).toBeDefined();
    expect(
      screen.queryByRole("button", { name: /Look up contact/ }),
    ).toBeNull();
  });

  it("offers a re-check over a merged record whose runs came from the other side", async () => {
    // After a merge the survivor's claims are relinked from runs that were
    // never its own, which is what contributing_runs carries.
    mount(
      { ...neverRun(), contributing_runs: [completedProviderRun] },
      queuedRun,
    );

    expect(
      await screen.findByRole("button", { name: /Check again/ }),
    ).toBeDefined();
  });
});

describe("two providers connected", () => {
  it("names each section and keeps one provider's values out of the other", async () => {
    const bought = { ...providerCompletedProfile, provider: "surfe" as const };
    const unasked = { ...neverRun(), provider: "acmedata" as const };
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "GET /contacts/p-1/enrichment-runs/run-1": () =>
        jsonResponse({ ...completedProviderRun, state: "completed" }),
    });
    render(
      <StoryProviders>
        <ContactProviderSection contactId="p-1" profiles={[bought, unasked]} />
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
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "POST /contacts/p-1/enrichment-runs": (body) => {
        posted.push(body);
        return queuedRun();
      },
    });
    render(
      <StoryProviders>
        <ContactProviderSection
          contactId="p-1"
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
      name: /Look up contact/,
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
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "POST /contacts/p-1/enrichment-runs": () => {
        paths.push("p-1");
        return queuedRun();
      },
      "POST /contacts/p-2/enrichment-runs": () => {
        paths.push("p-2");
        return queuedRun();
      },
    });
    const { rerender } = render(
      <StoryProviders>
        <ContactProviderSection contactId="p-1" profiles={[neverRun()]} />
      </StoryProviders>,
    );
    await screen.findByRole("button", { name: /Look up contact/ });

    // The panel stays mounted across the change of subject: the record page
    // keys its subtree by contact today, and this is the case that breaks the
    // moment it stops.
    rerender(
      <StoryProviders>
        <ContactProviderSection contactId="p-2" profiles={[neverRun()]} />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: /Look up contact/ }),
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
    expect(await screen.findByText(/1 of 5 details returned/)).toBeDefined();
  });

  it("names the categories the provider had nothing for, in words a rep knows", async () => {
    mount(mostlyEmpty(), queuedRun);

    const line = await screen.findByText(/Requested, not found/);
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
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "GET /contacts/p-1/enrichment-runs/run-live": () =>
        jsonResponse(inFlight()),
    });
    render(
      <StoryProviders>
        <ContactProviderSection
          contactId="p-1"
          profiles={[
            { ...neverRun(), state: "provider_error", latest_run: inFlight() },
          ]}
        />
      </StoryProviders>,
    );

    expect(await screen.findByText(/Querying Surfe/)).toBeDefined();
    // The connection's last failure is real and belongs on the page at rest.
    // Over a lookup the reader is watching it is a stale fact dressed as a
    // live one, which is what sent Lars looking for a broken button.
    expect(screen.queryByText(/last call to the provider failed/)).toBeNull();
    expect(await screen.findByText("Looking up…")).toBeDefined();
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
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "GET /contacts/p-1/enrichment-runs/run-landing": () =>
        jsonResponse(unapplied),
    });
    render(
      <StoryProviders>
        <ContactProviderSection
          contactId="p-1"
          profiles={[
            { ...neverRun(), state: "completed", latest_run: unapplied },
          ]}
        />
      </StoryProviders>,
    );

    expect(await screen.findByText("Response received")).toBeDefined();
    expect(screen.queryByText(/Querying Surfe/)).toBeNull();
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
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "GET /contacts/p-1/enrichment-runs/run-landing-2": () =>
        jsonResponse(unapplied),
    });
    render(
      <StoryProviders>
        <ContactProviderSection
          contactId="p-1"
          profiles={[
            { ...neverRun(), state: "completed", latest_run: unapplied },
          ]}
        />
      </StoryProviders>,
    );

    await screen.findByText("Response received");
    expect(screen.queryByRole("button", { name: /Buy / })).toBeNull();
    expect(screen.queryByRole("button", { name: /Check again/ })).toBeNull();
  });

  it("polls a live run even while the connection reads as failed", async () => {
    let polls = 0;
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "GET /contacts/p-1/enrichment-runs/run-live": () => {
        polls += 1;
        return jsonResponse(inFlight());
      },
    });
    render(
      <StoryProviders>
        <ContactProviderSection
          contactId="p-1"
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
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "GET /contacts/p-1/enrichment-runs/run-unapplied": () => {
        polls += 1;
        return jsonResponse(unapplied);
      },
    });
    render(
      <StoryProviders>
        <ContactProviderSection
          contactId="p-1"
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
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "GET /contacts/p-1/enrichment-runs/run-1": () => {
        polls += 1;
        return jsonResponse(completedProviderRun);
      },
    });
    render(
      <StoryProviders>
        <ContactProviderSection
          contactId="p-1"
          profiles={[providerCompletedProfile]}
        />
      </StoryProviders>,
    );

    await screen.findByRole("button", { name: /Check again/ });
    expect(polls).toBe(0);
  });
});
