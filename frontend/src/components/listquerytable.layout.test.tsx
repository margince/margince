/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { type ListPage, ListTable, useListQuery } from "./listquery";

// Two record lists that count the same unit keep their own column layout.

type Row = { id: string; name: string; city: string; stage: string };

function onePage(): Promise<ListPage<Row>> {
  return Promise.resolve({
    data: [{ id: "r1", name: "Ada", city: "Berlin", stage: "Lead" }],
    page: { next_cursor: null, has_more: false },
  });
}

function Contacts({ layoutKey }: Readonly<{ layoutKey?: string }>) {
  const state = useListQuery<Row>({
    key: `layout-${layoutKey ?? "unit"}`,
    fetchPage: onePage,
  });
  return (
    <ListTable
      state={state}
      unit="unit.contacts"
      layoutKey={layoutKey}
      columns={[
        {
          key: "name",
          header: "Name",
          cell: (row: Row) => row.name,
          fixed: true,
        },
        { key: "city", header: "City", cell: (row: Row) => row.city },
        { key: "stage", header: "Stage", cell: (row: Row) => row.stage },
      ]}
      rowKey={(row) => row.id}
    />
  );
}

function mount(layoutKey?: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Contacts layoutKey={layoutKey} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function headers(): Promise<string[]> {
  const table = await screen.findByRole("table");
  await within(table).findByText("Ada");
  return within(table)
    .getAllByRole("columnheader")
    .map((cell) => cell.textContent ?? "");
}

afterEach(() => {
  cleanup();
  localStorage.clear();
});

describe("a list's column layout", () => {
  it("is its own for two lists of one unit", async () => {
    const user = userEvent.setup();
    const main = mount();
    await headers();
    await user.click(screen.getByRole("button", { name: "Display" }));
    screen.getByRole("button", { name: "Move Stage, 2 of 2" }).focus();
    await user.keyboard("{ArrowUp}");
    expect(await headers()).toEqual(["Name", "Stage", "City"]);
    main.unmount();

    const company = mount("company-contacts");
    expect(await headers()).toEqual(["Name", "City", "Stage"]);
    company.unmount();

    mount();
    expect(await headers()).toEqual(["Name", "Stage", "City"]);
    // Three mounts, each waiting for its table and its row: a ceiling of its own
    // that covers those waiters.
  }, 25_000);
});
