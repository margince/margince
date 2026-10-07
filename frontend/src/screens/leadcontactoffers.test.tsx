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
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { LocaleProvider } from "../i18n";
import { LeadScreen, LeadsScreen } from "./leads";
import { jsonResponse } from "./story-utils";

type Contact = components["schemas"]["Contact"];

const ben: Partial<Contact> = {
  id: "c-ben",
  full_name: "Ben Sample",
  primary_email: "ben@contoso.example",
  title: "Head of Logistics",
  employer: { company_id: "co-1", company_name: "Contoso Ltd" },
};

const anna: Partial<Contact> = {
  id: "c-anna",
  full_name: "Anna Example",
  primary_email: "anna@northwind.example",
  employer: { company_id: "co-2", company_name: "Northwind Traders" },
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
      if (url.endsWith("/v1/lead-sources")) {
        return jsonResponse({
          data: [
            {
              id: "src-manual",
              key: "manual",
              label: "Created manually",
              intent: "neutral",
              sort_order: 10,
              active: true,
              system: true,
              lead_count: 0,
              version: 1,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            },
          ],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.includes("/contacts?")) {
        return jsonResponse({
          data: [ben, anna],
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

describe("a lead's identity, offered from the contacts", () => {
  it("fills the new lead from the contact picked by name, and names the contact to the server", async () => {
    const writes: { method: string; body: unknown }[] = [];
    stubBackend(writes);
    const user = userEvent.setup();
    mount(<LeadsScreen />);

    await user.click(await screen.findByTestId("new-record"));
    const form = within(screen.getByRole("dialog"));
    await user.type(form.getByRole("combobox", { name: /Full name/ }), "Ben");
    await user.click(await screen.findByRole("option", { name: /Ben Sample/ }));

    expect(form.getByRole("combobox", { name: "Email" })).toHaveValue(
      "ben@contoso.example",
    );
    expect(form.getByLabelText("Company")).toHaveValue("Contoso Ltd");
    await user.click(form.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]?.body).toMatchObject({
      full_name: "Ben Sample",
      email: "ben@contoso.example",
      title: "Head of Logistics",
      company_name: "Contoso Ltd",
      contact_id: "c-ben",
    });
  });

  it("replaces the first person when a second is picked, leaving nothing of them behind", async () => {
    const writes: { method: string; body: unknown }[] = [];
    stubBackend(writes);
    const user = userEvent.setup();
    mount(<LeadsScreen />);

    await user.click(await screen.findByTestId("new-record"));
    const form = within(screen.getByRole("dialog"));
    await user.type(form.getByRole("combobox", { name: /Full name/ }), "Ben");
    await user.click(await screen.findByRole("option", { name: /Ben Sample/ }));
    const email = form.getByRole("combobox", { name: "Email" });
    await user.clear(email);
    await user.type(email, "anna");
    await user.click(
      await screen.findByRole("option", { name: /anna@northwind/ }),
    );
    await user.click(form.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]?.body).toMatchObject({
      full_name: "Anna Example",
      email: "anna@northwind.example",
      company_name: "Northwind Traders",
      contact_id: "c-anna",
    });
    expect(writes[0]?.body).not.toHaveProperty("title");
  });

  it("fills an unnamed lead from a contact in one save", async () => {
    const writes: { method: string; body: unknown }[] = [];
    stubBackend(writes);
    const user = userEvent.setup();
    mount(
      <RecordShell>
        <LeadScreen id="l-1" />
      </RecordShell>,
    );

    await user.click(
      await screen.findByRole("button", { name: "Fill from a contact" }),
    );
    await user.type(
      screen.getByRole("combobox", {
        name: "Search contacts by name or email",
      }),
      "ben",
    );
    await user.click(await screen.findByRole("option", { name: /Ben Sample/ }));

    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toMatchObject({
      method: "PATCH",
      body: {
        full_name: "Ben Sample",
        email: "ben@contoso.example",
        title: "Head of Logistics",
        company_name: "Contoso Ltd",
      },
    });
  });
});
