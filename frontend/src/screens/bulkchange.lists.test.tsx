// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { CompaniesScreen } from "./companies";
import { ContactsScreen } from "./contacts";

// The contacts and companies lists offer the same two bulk verbs over a
// selection, and both open the preview before anything is written. The row a
// reader ticks is the row the preview names, at the version the list showed.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
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

/** Serves one list page at `listPath`, the roster, and an empty preview. */
function stubList(listPath: string, rows: readonly unknown[]) {
  const previews: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const path = new URL(input.url, "https://test.local").pathname;
      if (path === "/v1/bulk/preview") {
        previews.push(await input.clone().json());
        return json({
          record_type: "contact",
          verb: "archive",
          count: 0,
          affected: [],
          excluded: [],
          sample: [],
          requires_confirmation: false,
        });
      }
      if (path === listPath) {
        return json({ data: rows, page: { next_cursor: null } });
      }
      return json({ data: [], page: { next_cursor: null } });
    }),
  );
  return previews;
}

const cases = [
  {
    screen: "contacts",
    ui: <ContactsScreen />,
    listPath: "/v1/contacts",
    recordType: "contact",
    row: {
      id: "c-1",
      full_name: "Anna Weber",
      captured_by: "human:u-1",
      source: "manual",
      version: 4,
    },
    name: "Anna Weber",
  },
  {
    screen: "companies",
    ui: <CompaniesScreen />,
    listPath: "/v1/companies",
    recordType: "company",
    row: {
      id: "co-1",
      display_name: "Brandt GmbH",
      captured_by: "human:u-1",
      source: "manual",
      version: 9,
    },
    name: "Brandt GmbH",
  },
] as const;

describe.each(cases)("the $screen list's bulk bar", (entry) => {
  it("appears once a row is selected, and archive previews that row at its version", async () => {
    const previews = stubList(entry.listPath, [entry.row]);
    const user = userEvent.setup();
    render(entry.ui);

    const tick = await screen.findByRole("checkbox", {
      name: en["bulk.selectRow"].replace("{name}", entry.name),
    });
    expect(
      screen.queryByRole("button", { name: en["bulk.archive"] }),
    ).toBeNull();

    await user.click(tick);
    expect(
      screen.getByRole("combobox", { name: en["bulk.owner"] }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: en["bulk.archive"] }));

    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText(en["bulk.nothing"]),
    ).toBeInTheDocument();
    await waitFor(() => expect(previews).toHaveLength(1));
    expect(previews[0]).toEqual({
      record_type: entry.recordType,
      verb: "archive",
      items: [{ id: entry.row.id, version: entry.row.version }],
    });
  });
});
