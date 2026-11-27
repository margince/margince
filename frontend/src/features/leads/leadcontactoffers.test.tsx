/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { RecordShell } from "../../testing/recordshell.testkit";
import { jsonResponse } from "../../testing/story-utils";
import { LeadScreen, LeadsScreen } from "./leads";

type Contact = components["schemas"]["Contact"];

// A contact the search would return if the form still asked it.
const ben: Partial<Contact> = {
  id: "c-ben",
  full_name: "Ben Sample",
  primary_email: "ben@contoso.example",
  employer: { company_id: "co-1", company_name: "Contoso Ltd" },
};

const unnamedLead = {
  id: "l-1",
  company_name: "Contoso Ltd",
  status: "new",
  score: 0,
  source: "manual",
  captured_by: "human:u-me",
  owner_id: "u-me",
  version: 3,
  writable: true,
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

/** The backend both screens read; every write is recorded by method. */
function stubBackend(writes: { method: string; body: unknown }[]) {
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = request ? request.url : String(input);
      const method = request?.method ?? init?.method ?? "GET";
      if (method === "POST" || method === "PATCH") {
        const text = request ? await request.text() : String(init?.body ?? "");
        writes.push({ method, body: JSON.parse(text) });
        return jsonResponse(
          { ...unnamedLead, full_name: "Ben Sample" },
          method === "POST" ? 201 : 200,
        );
      }
      if (url.includes("/me")) {
        return jsonResponse({
          user: { id: "u-me", email: "me@example.test", display_name: "Me" },
          roles: ["rep"],
          teams: [],
          authorization: meFixture({
            allow: {
              lead: ["read", "create", "update"],
              contact: ["read"],
              activity: ["read", "create"],
            },
          }).authorization,
        });
      }
      if (url.includes("/contacts?")) {
        return jsonResponse({
          data: [ben],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (/\/leads\/l-1(\?|$)/.test(url)) {
        return jsonResponse(unnamedLead);
      }
      return jsonResponse({
        data: [],
        page: { next_cursor: null, has_more: false, total: 0 },
      });
    },
  );
}

function mount(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("a lead's identity is typed, never offered from the contacts", () => {
  it("offers no contact while the name and email of a new lead are typed", async () => {
    const writes: { method: string; body: unknown }[] = [];
    stubBackend(writes);
    const user = userEvent.setup();
    mount(<LeadsScreen />);

    await user.click(await screen.findByTestId("new-record"));
    const form = within(screen.getByRole("dialog"));
    // A plain box: a combobox is how the form offers records to pick.
    expect(form.queryByRole("combobox", { name: /Full name/ })).toBeNull();
    expect(form.queryByRole("combobox", { name: "Email" })).toBeNull();
    await user.type(form.getByLabelText(/Full name/), "Ben");
    await user.type(form.getByLabelText("Email"), "ben@contoso.example");
    await user.click(form.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]?.body).toMatchObject({
      full_name: "Ben",
      email: "ben@contoso.example",
    });
    expect(writes[0]?.body).not.toHaveProperty("contact_id");
  });

  it("offers no Fill from a contact on a lead", async () => {
    stubBackend([]);
    const user = userEvent.setup();
    mount(
      <RecordShell>
        <LeadScreen id="l-1" />
      </RecordShell>,
    );

    await user.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    await screen.findByRole("button", { name: "Share" });
    expect(
      screen.queryByRole("button", { name: "Fill from a contact" }),
    ).toBeNull();
  });
});
