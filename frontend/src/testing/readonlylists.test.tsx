// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ComponentType } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CompaniesScreen } from "../features/companies/companies";
import { ContactsScreen } from "../features/contacts/contacts";
import { DealsScreen } from "../features/deals/deals";
import { LeadsScreen } from "../features/leads/leads";
import { ProjectsScreen } from "../features/projects/projects";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meFixture } from "./mefixture";

// The seeded Read-only role on each record list: every create the list offers
// is refused before a form opens, with the sentence that names the role. The
// server refuses each of these writes, so a form that opened would be filled
// in for nothing.

const READ_ONLY = meFixture({
  roles: ["read_only"],
  rowScope: "all",
  allow: {
    contact: ["read"],
    company: ["read"],
    lead: ["read"],
    deal: ["read"],
    project: ["read"],
    activity: ["read"],
  },
});

const PIPELINE = {
  id: "pl",
  name: "Sales",
  is_default: true,
  position: 0,
  stages: [
    {
      id: "s1",
      pipeline_id: "pl",
      name: "Qualify",
      position: 1,
      semantic: "open",
      win_probability: 20,
    },
  ],
};

type ListScreen = ComponentType<{ startCreating?: boolean }>;

const LISTS: readonly (readonly [string, ListScreen, readonly string[]])[] = [
  [
    "Contacts",
    ContactsScreen,
    [en["create.quickCapture"], en["create.contact"], en["vcardImport.action"]],
  ],
  ["Companies", CompaniesScreen, [en["create.company"]]],
  ["Leads", LeadsScreen, [en["create.lead"]]],
  ["Deals", DealsScreen, [en["create.deal"]]],
  ["Projects", ProjectsScreen, [en["project.new"]]],
];

function mount(Screen: ListScreen) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      // New deal needs a pipeline to file the deal in.
      const data = new URL(request.url).pathname.endsWith("/pipelines")
        ? [PIPELINE]
        : [];
      return new Response(
        JSON.stringify({ data, page: { next_cursor: null } }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], READ_ONLY);
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The list's own create address: the form would open on arrival. */}
        <Screen startCreating />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe.each(LISTS)(
  "the %s list under the Read-only role",
  (_, Screen, verbs) => {
    it("refuses every create with the role's sentence, and opens no form", async () => {
      mount(Screen);

      for (const verb of verbs) {
        const button = await screen.findByRole("button", {
          name: new RegExp(verb),
        });
        await waitFor(() => expect(button).toBeDisabled());
        expect(button).toHaveAccessibleDescription(en["record.actionRefused"]);
      }
      expect(screen.queryByRole("dialog")).toBeNull();
    });
  },
);
