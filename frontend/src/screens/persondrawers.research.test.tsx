/** @vitest-environment jsdom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { PersonResearchDrawer } from "./persondrawers";
import { providerCompletedProfile } from "./personprovider.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Two capabilities share this drawer: contact data BOUGHT from a licensed
// provider at the top, and a public-source research run under it. Each has its
// own connection state, and for a while both spoke about "a data provider" — so
// a reader with Surfe connected and eight purchased claims on screen read, one
// paragraph below them, that no data provider was connected.
//
// The invariant this pins is a vocabulary one: the research empty state names
// the thing that is actually missing, and leaves "data provider" to the section
// that means the licensed contact-data sense.

afterEach(() => {
  cleanup();
});

function mountWithBoughtData() {
  installFetchStub({
    "GET /me": meRoute({ person: ["read"] }),
    "POST /people/p-1/research": () =>
      jsonResponse({
        person_id: "p-1",
        state: "not_connected",
        generated_at: "2026-08-18T09:00:00Z",
        claims: [],
      }),
  });
  render(
    <StoryProviders>
      <PersonResearchDrawer
        personId="p-1"
        personName="Dana Buyer"
        providerProfiles={[providerCompletedProfile]}
        open
        onClose={() => undefined}
      />
    </StoryProviders>,
  );
}

describe("the research drawer with no research provider bound", () => {
  it("names the research provider as what is missing, not the data provider whose purchase is on screen", async () => {
    mountWithBoughtData();

    // The purchase, above: what a licensed contact-data provider was paid for,
    // under that provider's own name. The heading is the vendor rather than a
    // generic title, which is what makes the contradiction below visible — the
    // drawer denies a connection while showing Surfe's purchase.
    expect(await screen.findByText("Surfe")).toBeDefined();

    const empty = await screen.findByText(/No research provider is connected/);
    // "data provider" belongs to the section above. Reusing it here is the
    // contradiction — the drawer would deny the connection whose fruit it is
    // displaying.
    expect(empty.textContent ?? "").not.toContain("data provider");
  });

  it("still says Margince does not research a person on its own authority", async () => {
    // The reword changes which capability the sentence is about, not the promise
    // it carries: no provider means no crawl, and that is a guarantee of the
    // research port itself rather than a line about licensed contact data.
    mountWithBoughtData();

    expect(
      await screen.findByText(
        /never researches a contact on its own authority/,
      ),
    ).toBeDefined();
  });
});

// A ready run's ONE claim, carrying a source with a verbatim quote and an http
// URL — the evidence the save endpoint refuses a claim for lacking.
const acmeClaim = {
  ordinal: 1,
  body: "Head of Procurement at Acme",
  confidence: "high" as const,
  sources: [
    {
      label: "Acme team page",
      url: "https://acme.example/team",
      quote: "Dana Buyer, Head of Procurement",
    },
  ],
};

// The ready state a research provider IS bound reaches — unreachable to browser
// QC without a licensed provider, but the run is stubbed here, so the accepting
// half the drawer never had a test for is exercisable.
function mountReady(saved: unknown[], onClose: () => void = () => undefined) {
  installFetchStub({
    "GET /me": meRoute({ person: ["read"] }),
    "POST /people/p-1/research": () =>
      jsonResponse({
        person_id: "p-1",
        state: "ready",
        provider_name: "Clearbit",
        generated_at: "2026-08-18T09:00:00Z",
        sources_read: 2,
        claims: [acmeClaim],
      }),
    "POST /people/p-1/research/save": (body) => {
      saved.push(body);
      return jsonResponse({ saved: 1 });
    },
  });
  render(
    <StoryProviders>
      <ToastProvider>
        <PersonResearchDrawer
          personId="p-1"
          personName="Dana Buyer"
          open
          onClose={onClose}
        />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>,
  );
}

describe("the research drawer mapping claims to profile fields", () => {
  it("posts only the claim a reader mapped, never an empty array", async () => {
    // The bug this pins: the mutation shipped `body: { claims: [] }`, so a Save
    // stored nothing and said nothing. Nothing in the tree ever asserted on the
    // posted body, which is the only reason an empty write could survive.
    const user = userEvent.setup();
    const saved: unknown[] = [];
    mountReady(saved);

    await screen.findByText(acmeClaim.body);

    await user.click(screen.getByRole("combobox", { name: /profile field/i }));
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", { name: "Role" }),
    );
    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => expect(saved).toHaveLength(1));
    expect(saved[0]).toEqual({
      claims: [
        {
          field: "role",
          value: acmeClaim.body,
          source_quote: acmeClaim.sources[0].quote,
          source_url: acmeClaim.sources[0].url,
        },
      ],
    });
  });

  it("cannot save until a claim is mapped, then confirms and closes", async () => {
    // The other half of the defect: the button was always enabled and never
    // confirmed. Now it is offered only once a claim maps to a field, and a
    // save says how many landed and closes the drawer over the record.
    const user = userEvent.setup();
    const saved: unknown[] = [];
    let closed = false;
    mountReady(saved, () => {
      closed = true;
    });

    await screen.findByText(acmeClaim.body);
    expect(
      screen
        .getByRole("button", { name: /review & save/i })
        .hasAttribute("disabled"),
    ).toBe(true);

    await user.click(screen.getByRole("combobox", { name: /profile field/i }));
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", { name: "Role" }),
    );
    await user.click(screen.getByRole("button", { name: /review & save/i }));

    expect(await screen.findByText(/added to the record/i)).toBeDefined();
    await waitFor(() => expect(closed).toBe(true));
    expect(saved).toHaveLength(1);
  });
});
