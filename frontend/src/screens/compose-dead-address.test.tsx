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

  it("says nothing about an address the ledger does not mark", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse({
          full_name: "Anna Weiss",
          first_name: "Anna",
          address: "anna@live.example",
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

    expect(await screen.findByText("anna@live.example")).toBeTruthy();
    await waitFor(() => {
      expect(screen.queryByText(/is bouncing/)).toBeNull();
    });
  });

  it("says nothing when the caller may not read the send ledger", async () => {
    stubRoutes({
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

    expect(await screen.findByText("anna@dead.example")).toBeTruthy();
    await waitFor(() => {
      expect(screen.queryByText(/is bouncing/)).toBeNull();
    });
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
    expect(await screen.findByText("anna@dead.example")).toBeTruthy();
    await waitFor(() => {
      expect(seen.some((key) => key.includes("/360"))).toBe(false);
    });
  });
});
