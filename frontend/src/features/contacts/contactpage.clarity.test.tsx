/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { mount, view } from "../../testing/contactpage.testkit";
import { jsonResponse } from "../../testing/story-utils";

const sparse: components["schemas"]["Contact360"] = {
  as_of: view.as_of,
  contact: { ...view.contact, primary_email: "dana@brandt.example" },
  sections_omitted: [],
  activities: { data: [], page: { has_more: false } },
  network: { colleagues: [] },
  moment: {
    rule: "thin_relationship",
    headline: "No interactions recorded",
    why_now: "No interactions were found in available records.",
    claim_key: "thin",
    evidence_fingerprint: "empty",
    confidence: "observed_fact",
    evidence: [],
    recommended_action: {
      kind: "log_activity",
      label: "Log an interaction",
      state: "available",
      destination: { surface: "activity_log" },
    },
  },
};
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a contact with little context", () => {
  it("leads with recorded facts without fabricated work or sources", async () => {
    mount("overview", sparse);
    expect(
      await screen.findByRole("heading", { name: /^How it stands/ }),
    ).toBeTruthy();
    // The Next bar answers where the question is asked. On a thin
    // relationship the answer is what the server found and how far the
    // reading reached. It never says "nothing is due", a claim about a
    // record nobody has written anything about. It never shows a move read
    // out of records that are not there.
    expect(screen.queryByText(en["standing.next.none"])).toBeNull();
    expect(
      screen.getAllByText("No interactions recorded").length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText(en["today.quiet"])).toBeNull();
    expect(screen.getByText(en["contact.overview.coverage"])).toBeTruthy();
    for (const label of [
      en["co.suggest.byline"],
      en["co.suggest.basedOn"],
      "One thread only",
      "Nothing captured, nobody connected",
      "One-sided",
    ]) {
      expect(screen.queryByText(label)).toBeNull();
    }
    expect(screen.queryByRole("heading", { name: "Consent" })).toBeNull();
  });

  it("names who wrote the record on the Research tab's details", async () => {
    const user = userEvent.setup();
    mount("research", sparse);
    const label = await screen.findByText("Source", {
      selector: ".fieldgrid-label",
    });
    const sourceFact = label.nextElementSibling;
    if (!(sourceFact instanceof HTMLElement)) {
      throw new Error("the Source fact is not on the page");
    }
    await user.click(within(sourceFact).getByRole("button"));
    expect(screen.getByText("manual")).toBeTruthy();
  });

  it("keeps a failed permission read distinct from successful emptiness, and retries", async () => {
    const user = userEvent.setup();
    let guardReads = 0;
    mount("overview", sparse, [], {
      "GET /contacts/p-1/consent/guard": () =>
        ++guardReads === 1
          ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
          : jsonResponse({ contact_id: "p-1", entries: [] }),
    });
    const alert = await screen.findByRole("alert");
    await user.click(within(alert).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(guardReads).toBe(2);
  });

  it("loads the ledger only when requested and refreshes the column's guard after recording consent", async () => {
    const user = userEvent.setup();
    let ledgerReads = 0;
    let guardReads = 0;
    let recorded = false;
    const entry = {
      purpose_id: "p1",
      purpose_key: "business",
      state: "unknown",
    };
    mount("research", sparse, [], {
      "GET /consent-purposes": () =>
        jsonResponse({
          data: [
            {
              id: "p1",
              key: "business",
              label: "Business correspondence",
              requires_double_opt_in: false,
            },
          ],
          page: { has_more: false },
        }),
      "GET /contacts/p-1/consent": () => {
        ledgerReads++;
        return jsonResponse({
          state: [{ ...entry, state: recorded ? "granted" : "unknown" }],
          events: [],
        });
      },
      "POST /contacts/p-1/consent": () => {
        recorded = true;
        return jsonResponse({});
      },
      "GET /contacts/p-1/consent/guard": () => {
        guardReads++;
        return jsonResponse({
          contact_id: "p-1",
          entries: [
            {
              purpose_key: "business",
              purpose_class: "business_correspondence",
              purpose_label: "Business correspondence",
              channel: "email",
              verdict: recorded ? "allowed" : "unknown",
            },
          ],
        });
      },
    });
    const manage = await screen.findByRole("button", {
      name: "Manage consent and proof history",
    });
    expect(ledgerReads).toBe(0);
    expect(screen.getAllByTestId("confirm-details-ask")).toHaveLength(1);
    expect(screen.getByText("Send to dana@brandt.example")).toBeTruthy();
    await user.click(manage);
    const dialog = await screen.findByRole("dialog", {
      name: "Manage consent and proof history",
    });
    await user.click(
      await within(dialog).findByRole("button", { name: "Record consent" }),
    );
    await waitFor(() => expect(guardReads).toBeGreaterThan(1));
    expect(await within(dialog).findByText("granted")).toBeTruthy();
    expect(within(dialog).queryByTestId("confirm-details-ask")).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    // The left column and the Research tab's panel read the one guard.
    expect((await screen.findAllByText("Allowed")).length).toBe(2);
  });

  it("does not offer a confirmation send on a read-only record", async () => {
    mount("research", {
      ...sparse,
      contact: { ...sparse.contact, writable: false },
    });
    await screen.findByRole("button", {
      name: "Manage consent and proof history",
    });
    expect(screen.queryByTestId("confirm-details-ask")).toBeNull();
  });

  it("explains why confirmation cannot be sent without an email", async () => {
    mount("research", {
      ...sparse,
      contact: { ...sparse.contact, primary_email: null, emails: [] },
    });
    const ask = await screen.findByTestId("confirm-details-ask");
    expect(ask).toHaveProperty("disabled", true);
    expect(screen.getAllByText("No address on file").length).toBeGreaterThan(0);
  });
});

it("names restricted overview sections even when an open commitment leads", async () => {
  mount("overview", {
    ...sparse,
    moment: undefined,
    sections_omitted: [
      "activities",
      "conversation_memory",
      "last_touch",
      "commercial",
      "next_meeting",
      "moments",
      "next_steps",
    ],
    claims: [
      {
        id: "claim-1",
        kind: "commitment_ours",
        body: "Send the pilot quote",
        source_activity_id: "a-1",
        source_quote: "I will send the quote.",
        status: "open",
        needs_review: false,
      },
    ],
  });
  expect(
    await screen.findByRole("checkbox", { name: /Send the pilot quote/ }),
  ).toBeTruthy();
  // Each restricted card says so itself instead of a notice naming them all.
  expect(screen.getAllByText("Not shown").length).toBeGreaterThan(0);
  expect(screen.queryByText("No interactions recorded")).toBeNull();
  expect(screen.queryByText("Nothing needs you today")).toBeNull();
});

it("draws the details column beside the feed, with no switch to hide it", async () => {
  mount("overview", sparse);
  expect(
    await screen.findByRole("heading", { name: "Consent and channels" }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: /details and permissions/ }),
  ).toBeNull();
});

it("does not predict a confirmation destination when multiple addresses have no selected primary", async () => {
  const first = sparse.contact.emails?.[0];
  if (!first) throw new Error("The fixture needs an email");
  mount("research", {
    ...sparse,
    contact: {
      ...sparse.contact,
      emails: [
        { ...first, is_primary: false },
        {
          ...first,
          id: "e-2",
          email: "second@example.test",
          is_primary: false,
          position: 1,
        },
      ],
    },
  });
  expect(await screen.findByTestId("confirm-details-ask")).toHaveProperty(
    "disabled",
    false,
  );
  expect(screen.queryByText(/Send to /)).toBeNull();
});
