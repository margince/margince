/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { ContactRail } from "./contactrail";

// The About column's Employer row, driven through the rail that draws it. What
// is asserted is the requests sent, since they are the whole contract.

type Contact360 = components["schemas"]["Contact360"];
type Employment = components["schemas"]["Contact360Employment"];

const NOW = new Date("2026-08-18T09:00:00Z");
const page = { has_more: false, next_cursor: null };

const brandt: Employment = {
  relationship_id: "rel-1",
  company_id: "o-1",
  company_name: "Brandt Automotive GmbH",
  role: "Head of Fleet",
  is_current_primary: true,
  started_at: "2022-03-01T00:00:00Z",
  ended_at: null,
};

function viewWith(employments: Employment[], writable = true): Contact360 {
  return {
    as_of: NOW.toISOString(),
    contact: {
      id: "p-1",
      full_name: "Dana Buyer",
      first_name: "Dana",
      source: "manual",
      captured_by: "human:u-1",
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-08-01T08:00:00Z",
      writable,
    },
    sections_omitted: [],
    employments: { data: employments, page },
  };
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

const sent: Array<{ method: string; path: string; body?: unknown }> = [];

function mount(view: Contact360) {
  sent.length = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(request?.url ?? String(input), "https://test");
      const method = request?.method ?? "GET";
      if (method !== "GET") {
        const raw = request ? await request.clone().text() : "";
        sent.push({
          method,
          path: url.pathname,
          body: raw === "" ? undefined : JSON.parse(raw),
        });
      }
      if (url.pathname.endsWith("/me")) {
        return json(meFixture({ allow: { contact: ["read", "update"] } }));
      }
      // Every employment write pins If-Match, and the version comes from here.
      if (url.pathname.endsWith("/relationships") && method === "GET") {
        return json({
          data: (view.employments?.data ?? []).map((employment) => ({
            ...employment,
            id: employment.relationship_id,
            started_at: employment.started_at?.slice(0, 10),
            ended_at: employment.ended_at?.slice(0, 10),
            version: 3,
          })),
          page,
        });
      }
      if (url.pathname.endsWith("/companies")) {
        return json({
          data: [{ id: "o-9", display_name: "Employer GmbH" }],
          page,
        });
      }
      return json({ data: [], page });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactRail
          view={view}
          consent={{
            guard: undefined,
            loading: false,
            failed: false,
            onRetry: () => undefined,
          }}
          writeRefusalId="write-refused"
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function driver() {
  return userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
}

const writes = () => sent.filter((request) => request.path !== "/me");

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  vi.setSystemTime(NOW);
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  vi.unstubAllGlobals();
});

describe("the About column's employer", () => {
  it("moves the contact to another company and keeps the old one as former", async () => {
    mount(viewWith([brandt]));
    const user = driver();
    await user.click(
      await screen.findByRole("button", { name: "Change Employer" }),
    );
    await user.type(screen.getByRole("searchbox"), "emp");
    await user.click(await screen.findByText("Employer GmbH"));
    expect(
      screen.getByRole("radio", { name: /They left Brandt Automotive GmbH/ }),
    ).toHaveProperty("checked", true);
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(writes()).toHaveLength(2));
    expect(writes()).toEqual([
      {
        method: "POST",
        path: "/v1/relationships",
        body: {
          kind: "employment",
          contact_id: "p-1",
          company_id: "o-9",
          is_current_primary: true,
          source: "manual",
        },
      },
      {
        method: "PATCH",
        path: "/v1/relationships/rel-1",
        body: { ended_at: "2026-08-18" },
      },
    ]);
  });

  it("clears a wrong employer by removing the link, and writes no new one", async () => {
    mount(viewWith([brandt]));
    const user = driver();
    await user.click(
      await screen.findByRole("button", { name: "Change Employer" }),
    );
    await user.click(
      screen.getByRole("checkbox", { name: "No current employer" }),
    );
    await user.click(
      screen.getByRole("radio", {
        name: /Brandt Automotive GmbH was never their employer/,
      }),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(writes()).toHaveLength(1));
    expect(writes()[0]).toMatchObject({
      method: "DELETE",
      path: "/v1/relationships/rel-1",
    });
  });

  it("makes a job the contact already holds the primary one instead of adding a second", async () => {
    const other: Employment = {
      relationship_id: "rel-2",
      company_id: "o-9",
      company_name: "Employer GmbH",
      is_current_primary: false,
      ended_at: null,
    };
    mount(viewWith([brandt, other]));
    const user = driver();
    await user.click(
      await screen.findByRole("button", { name: "Change Employer" }),
    );
    await user.type(screen.getByRole("searchbox"), "emp");
    await user.click(await screen.findByText("Employer GmbH"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(writes()).toHaveLength(2));
    expect(writes()[0]).toEqual({
      method: "PATCH",
      path: "/v1/relationships/rel-2",
      body: { is_current_primary: true },
    });
    expect(writes().some((request) => request.method === "POST")).toBe(false);
  });

  it("asks nothing and saves nothing until the employer changes", async () => {
    mount(viewWith([brandt]));
    const user = driver();
    await user.click(
      await screen.findByRole("button", { name: "Change Employer" }),
    );
    expect(screen.queryByRole("radio")).toBeNull();
    expect(screen.getByRole("button", { name: "Save" })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("opens the add dialog for a contact who works nowhere", async () => {
    mount(viewWith([]));
    const user = driver();
    await user.click(await screen.findByText(/Show \d+ empty field/));
    await user.click(
      await screen.findByRole("button", { name: /Change Employer/ }),
    );
    expect(
      await screen.findByRole("checkbox", {
        name: "This is their current employer",
      }),
    ).toHaveProperty("checked", true);
  });

  it("stays read-only for a reader who cannot change the contact", async () => {
    mount(viewWith([brandt], false));
    await screen.findByText("Brandt Automotive GmbH");
    expect(
      screen.queryByRole("button", { name: "Change Employer" }),
    ).toBeNull();
  });
});
