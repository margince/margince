/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ComposeModal } from "./compose";
import { allowedPreview, isPreviewDoor } from "./sendpermission.testkit";

// The composer warns about an address that is known not to arrive.
//
// The person page badges one whose latest delivery hard-bounced with nothing
// clean since. Until now the mark lived only there — on a page the rep is not
// looking at while they write — so the one moment it could change a decision
// was the one moment it was absent.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
};

const THREAD_RECIPIENT = {
  full_name: "Anna Weiss",
  first_name: "Anna",
  address: "anna@dead.example",
};

// The 360 as the composer reads it: only the section it asks about matters,
// and `sections_omitted` is what a caller without the grant gets instead.
function person360(overrides: Record<string, unknown> = {}) {
  return {
    as_of: "2026-01-01T00:00:00Z",
    person: { id: "p-1", full_name: "Anna Weiss", source: "seed" },
    sections_omitted: [],
    ...overrides,
  };
}

function stubRoutes(
  overrides: Record<string, () => Response | Promise<Response>> = {},
) {
  const seen: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      seen.push(key);
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /consent-purposes") return jsonResponse(PURPOSES);
      if (key === "GET /voice-profiles") return jsonResponse({ data: [] });
      if (isPreviewDoor(url.pathname)) return jsonResponse(allowedPreview([]));
      return jsonResponse({});
    }),
  );
  return seen;
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("ComposeModal dead recipients", () => {
  it("warns when the prefilled recipient is an address that bounces", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
      "GET /people/p-1/360": () =>
        jsonResponse(person360({ dead_addresses: ["anna@dead.example"] })),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    expect(
      await screen.findByText(/anna@dead\.example is bouncing/),
    ).toBeTruthy();
  });

  it("warns about an address the reader types in a different case", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse({
          full_name: "Anna Weiss",
          first_name: "Anna",
          address: "",
        }),
      "GET /people/p-1/360": () =>
        jsonResponse(person360({ dead_addresses: ["anna@dead.example"] })),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(await screen.findByLabelText("To"));
    // The ledger holds what the provider reported; a rep types what they
    // remember. Comparing the two literally would warn about neither.
    await user.paste("Anna@Dead.Example");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText(/Anna@Dead\.Example is bouncing/),
    ).toBeTruthy();
  });

  // The live address is asserted absent from a warning that IS on screen, not
  // from a render that may simply not have finished. A `waitFor` over an
  // absence passes on the first tick, before the 360 has answered — so the
  // dead sibling is what proves this reader got as far as knowing.
  it("names only the address the ledger marks, not the one beside it", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse({
          full_name: "Anna Weiss",
          first_name: "Anna",
          address: "anna@dead.example",
        }),
      "GET /people/p-1/360": () =>
        jsonResponse(person360({ dead_addresses: ["anna@dead.example"] })),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(await screen.findByLabelText("To"));
    await user.paste("anna@live.example");
    await user.keyboard("{Enter}");

    const warning = await screen.findByText(/is bouncing/);
    expect(warning.textContent).toContain("anna@dead.example");
    expect(warning.textContent).not.toContain("anna@live.example");
  });

  it("says nothing when the caller may not read the send ledger", async () => {
    const seen = stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
      // Omitted, not empty: the two are different facts, and inventing a
      // warning from an absence would be a claim about correspondence this
      // reader may not see.
      "GET /people/p-1/360": () =>
        jsonResponse(person360({ sections_omitted: ["dead_addresses"] })),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    // The 360 has ANSWERED before this is asserted — the section is omitted,
    // not pending — so the absence is a decision rather than a render that had
    // not caught up.
    expect(await screen.findByText("anna@dead.example")).toBeTruthy();
    await waitFor(() => {
      expect(seen.filter((key) => key.endsWith("/360"))).toHaveLength(1);
    });
    expect(screen.queryByText(/is bouncing/)).toBeNull();
  });

  it("asks for no person page when the composer knows no person", async () => {
    const seen = stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="deal"
        entityId="d-1"
        open
        onClose={vi.fn()}
      />,
    );

    // A deal timeline names no single person, so there is nobody to ask about
    // — and asking anyway would spend a composite read on every open.
    //
    // Asserted once the composer has SETTLED: the recipient it fetched is on
    // screen, so every query this render was going to start has started. An
    // absence checked before that would pass on a request merely not made yet.
    expect(await screen.findByText("anna@dead.example")).toBeTruthy();
    await waitFor(() => {
      expect(seen).toContain("GET /activities/act-1/reply-recipient");
    });
    expect(seen.some((key) => key.includes("/360"))).toBe(false);
  });

  // ONE MENTION. To and Cc are asked about together, and a rep who has the same
  // address in both would otherwise read it named twice in a sentence about one
  // thing being wrong with it.
  it("names an address in both To and Cc once", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
      "GET /people/p-1/360": () =>
        jsonResponse(person360({ dead_addresses: ["anna@dead.example"] })),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    const user = userEvent.setup();
    await user.click(await screen.findByLabelText("Cc"));
    // A different case as well, since that is how the same address arrives
    // twice without looking like it.
    await user.paste("Anna@Dead.Example");
    await user.keyboard("{Enter}");

    const warning = await screen.findByText(/is bouncing/);
    const mentions = warning.textContent
      ?.toLowerCase()
      .split("anna@dead.example").length;
    if (mentions !== 2) {
      throw new Error(
        `the warning names the address ${(mentions ?? 1) - 1} times, want once: ${warning.textContent}`,
      );
    }
  });

  // THE ACCOUNT FLOW, which is the one with the most reason to warn: the rep is
  // choosing between an account's contacts rather than answering somebody who
  // already wrote, so the composer learns the person from the picker and not
  // from the record it was opened on.
  it("warns for the contact picked in an account draft", async () => {
    stubRoutes({
      "GET /organizations/org-1/360": () =>
        jsonResponse({
          state: "ready",
          as_of: "2026-01-01T00:00:00Z",
          organization: { id: "org-1", display_name: "Demo GmbH" },
          people: { data: [{ person_id: "p-1", full_name: "Anna Weiss" }] },
          deals: { data: [] },
          sections_omitted: [],
        }),
      "GET /people/p-1/360": () =>
        jsonResponse(person360({ dead_addresses: ["anna@dead.example"] })),
    });
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );

    const user = userEvent.setup();
    // Pick the contact, which is what tells this composer whose addresses to
    // ask about — before that it knows an organization and nobody.
    await user.click(await screen.findByRole("combobox", { name: "Draft to" }));
    await user.click(await screen.findByRole("option", { name: "Anna Weiss" }));
    await user.click(await screen.findByLabelText("To"));
    await user.paste("anna@dead.example");
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText(/anna@dead\.example is bouncing/),
    ).toBeTruthy();
  });

  // A channel reply renders no address fields at all — its recipient is
  // resolved server-side — so the warning has nowhere to go and the composite
  // read would buy nothing.
  it("asks for no person page for a channel reply, even knowing the person", async () => {
    const seen = stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
      "GET /people/p-1/360": () =>
        jsonResponse(person360({ dead_addresses: ["anna@dead.example"] })),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        personId="p-1"
        kind="message"
        open
        onClose={vi.fn()}
      />,
    );

    // Waited for the CHANNEL-REPLY's own read to land, not merely for the Send
    // control: the confirm dialog renders that while the reply is still
    // settling, so an absence asserted on it would pass before a re-enabled 360
    // query had reached fetch at all.
    await waitFor(() => {
      expect(seen).toContain("GET /activities/act-1");
    });
    expect(seen.some((key) => key.includes("/360"))).toBe(false);
  });
});
