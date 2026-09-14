/** @vitest-environment happy-dom */
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { UnsavedGuard } from "../app/unsaved";
import { mount, view } from "./contactpage.testkit";
import { jsonResponse } from "./story-utils";

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
    const user = userEvent.setup();
    mount("overview", sparse);
    expect(
      await screen.findByRole("heading", { name: "About this contact" }),
    ).toBeTruthy();
    expect(screen.getByText("No interactions recorded")).toBeTruthy();
    for (const label of [
      "Margince suggests",
      "One thread only",
      "What this rests on",
      "Nothing captured, nobody connected",
      "One-sided",
    ]) {
      expect(screen.queryByText(label)).toBeNull();
    }
    expect(screen.queryByRole("heading", { name: "Consent" })).toBeNull();
    expect(
      screen.queryByRole("heading", { name: "What needs you" }),
    ).toBeNull();
    await user.click(screen.getByRole("button", { name: /Source:/ }));
    expect(screen.getByText("manual")).toBeTruthy();
  });

  it("keeps failed brief and permission reads distinct from successful emptiness, and retries", async () => {
    const user = userEvent.setup();
    let briefReads = 0;
    let guardReads = 0;
    mount("overview", sparse, [], {
      "GET /contacts/p-1/brief": () =>
        ++briefReads === 1
          ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
          : jsonResponse({ contact_id: "p-1", sentences: [] }),
      "GET /contacts/p-1/consent/guard": () =>
        ++guardReads === 1
          ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
          : jsonResponse({ contact_id: "p-1", entries: [] }),
    });
    await screen.findAllByRole("alert");
    await waitFor(() => expect(screen.getAllByRole("alert")).toHaveLength(2));
    for (let attempt = 0; attempt < 2; attempt++) {
      await user.click(
        within(screen.getAllByRole("alert")[0]).getByRole("button", {
          name: "Retry",
        }),
      );
      await waitFor(() =>
        expect(screen.queryAllByRole("alert")).toHaveLength(1 - attempt),
      );
    }
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(
      await screen.findByRole("heading", { name: "About this contact" }),
    ).toBeTruthy();
  });

  it("loads the ledger only when requested and refreshes the sidebar guard after recording consent", async () => {
    const user = userEvent.setup();
    let ledgerReads = 0;
    let guardReads = 0;
    let recorded = false;
    const entry = {
      purpose_id: "p1",
      purpose_key: "business",
      state: "unknown",
    };
    mount("overview", sparse, [], {
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
      name: "Manage consent & proof history",
    });
    expect(ledgerReads).toBe(0);
    expect(screen.getAllByTestId("confirm-details-ask")).toHaveLength(1);
    expect(screen.getByText("Send to dana@brandt.example")).toBeTruthy();
    await user.click(manage);
    const dialog = await screen.findByRole("dialog", {
      name: "Manage consent & proof history",
    });
    await user.click(
      await within(dialog).findByRole("button", { name: "Record consent" }),
    );
    await waitFor(() => expect(guardReads).toBeGreaterThan(1));
    expect(await within(dialog).findByText("granted")).toBeTruthy();
    expect(within(dialog).queryByTestId("confirm-details-ask")).toBeNull();
    await user.click(within(dialog).getByRole("button", { name: "Close" }));
    expect(await screen.findByText("Allowed")).toBeTruthy();
  });

  it("does not offer a confirmation send on a read-only record", async () => {
    mount("overview", {
      ...sparse,
      contact: { ...sparse.contact, writable: false },
    });
    await screen.findByRole("button", {
      name: "Manage consent & proof history",
    });
    expect(screen.queryByTestId("confirm-details-ask")).toBeNull();
  });

  it("explains why confirmation cannot be sent without an email", async () => {
    mount("overview", {
      ...sparse,
      contact: { ...sparse.contact, primary_email: null, emails: [] },
    });
    const ask = await screen.findByTestId("confirm-details-ask");
    expect(ask).toHaveProperty("disabled", true);
    expect(screen.getAllByText("No address on file").length).toBeGreaterThan(0);
  });
});

it("opens details and permissions in a narrow-screen drawer and restores focus", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query === "(max-width: 720px)",
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));
  mount("overview", sparse);
  const toggle = await screen.findByRole("button", {
    name: "Details & permissions",
  });
  expect(
    screen.queryByRole("heading", { name: "Communication permissions" }),
  ).toBeNull();
  await user.click(toggle);
  const dialog = await screen.findByRole("dialog", {
    name: "Details & permissions",
  });
  expect(
    within(dialog).getByRole("heading", { name: "Communication permissions" }),
  ).toBeTruthy();
  expect(await within(dialog).findByTestId("confirm-details-ask")).toBeTruthy();
  await user.click(within(dialog).getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(toggle);
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
  expect(
    screen.getByText(
      /Not shown: Conversation memory, Where this contact stands, what Margince found, open tasks/,
    ),
  ).toBeTruthy();
  expect(screen.queryByText("No interactions recorded")).toBeNull();
  expect(screen.queryByText("Nothing needs you today")).toBeNull();
});

it("keeps the ordinary details pane at tablet width", async () => {
  vi.stubGlobal("matchMedia", () => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));
  mount("overview", sparse);
  expect(
    await screen.findByRole("heading", { name: "Communication permissions" }),
  ).toBeTruthy();
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("does not predict a confirmation destination when multiple addresses have no selected primary", async () => {
  const first = sparse.contact.emails?.[0];
  if (!first) throw new Error("The fixture needs an email");
  mount("overview", {
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

it("keeps a mobile Details edit open until it is saved or cancelled", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query === "(max-width: 720px)",
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));
  mount("overview", sparse, [], {}, undefined, (page) => (
    <UnsavedGuard address="p-1" onKeep={() => undefined}>
      {() => page}
    </UnsavedGuard>
  ));
  await user.click(
    await screen.findByRole("button", { name: "Details & permissions" }),
  );
  const dialog = await screen.findByRole("dialog", {
    name: "Details & permissions",
  });
  await user.click(
    await within(dialog).findByRole("button", { name: "Change Title" }),
  );
  const input = within(dialog).getByRole("textbox", { name: "Title" });
  await user.type(input, "Director");
  await waitFor(() =>
    expect(
      within(dialog)
        .getByRole("button", { name: "Close" })
        .hasAttribute("disabled"),
    ).toBe(true),
  );
  fireEvent.keyDown(document, { key: "Escape" });
  expect(within(dialog).getByDisplayValue("Director")).toBeTruthy();
  await user.keyboard("{Escape}");
  expect(
    screen.getByRole("dialog", { name: "Details & permissions" }),
  ).toBeTruthy();
  await user.click(within(dialog).getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});
