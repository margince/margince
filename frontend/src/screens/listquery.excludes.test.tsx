/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

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
import type { ListChip } from "../design-system/listsurface";
import { LocaleProvider } from "../i18n";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  useListQuery,
} from "./listquery";

// The server refuses owner_id, owner_team_id and unassigned together, so the
// Owner and Team dials each clear the other's parameter when one is picked.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Row = { id: string };

const OWNER: ListChip = {
  key: "owner",
  label: "Owner",
  allLabel: "Any owner",
  filterable: true,
  excludes: ["owner_team_id"],
  options: [
    { value: "owner_id:u-1", label: "Owned by you" },
    { value: "owner_id:u-2", label: "Anna Becker" },
    { value: "unassigned:true", label: "Unassigned" },
  ],
};

const TEAM: ListChip = {
  key: "owner_team",
  label: "Team",
  allLabel: "Any team",
  excludes: ["owner_id", "unassigned"],
  options: [{ value: "owner_team_id:t-1", label: "Region West" }],
};

function Harness({
  fetchPage,
  initialFilters,
}: Readonly<{
  fetchPage: (
    query: ListQuery,
    cursor: string | null,
  ) => Promise<ListPage<Row>>;
  initialFilters: Readonly<Record<string, string>>;
}>) {
  const state = useListQuery<Row>({
    key: "owner-dials-harness",
    initialSort: "-created_at",
    initialFilters,
    fetchPage,
  });
  return (
    <ListTable
      state={state}
      unit="nav.contacts"
      columns={[
        { key: "id", header: "contacts.name", cell: (row: Row) => row.id },
      ]}
      rowKey={(row) => row.id}
      dataChips={[OWNER, TEAM]}
    />
  );
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

function stubPage() {
  return vi.fn(async (_query: ListQuery, _cursor: string | null) => ({
    data: [] as Row[],
    page: { next_cursor: null, has_more: false },
  }));
}

describe("the Owner and Team dials", () => {
  it("swap rather than stack: picking a team drops the owner", async () => {
    const user = userEvent.setup();
    const fetchPage = stubPage();
    render(
      <Harness fetchPage={fetchPage} initialFilters={{ owner_id: "u-2" }} />,
    );
    await screen.findByRole("group", { name: "Owner: Anna Becker" });

    await user.click(screen.getByRole("button", { name: "Add filter" }));
    await user.click(screen.getByRole("button", { name: "Team" }));
    await user.click(screen.getByRole("radio", { name: "Region West" }));

    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].filters).toEqual({
        owner_team_id: "t-1",
      }),
    );
  });

  it("swap the other way: picking an owner drops the team", async () => {
    const user = userEvent.setup();
    const fetchPage = stubPage();
    render(
      <Harness
        fetchPage={fetchPage}
        initialFilters={{ owner_team_id: "t-1" }}
      />,
    );
    await screen.findByRole("group", { name: "Team: Region West" });

    await user.click(screen.getByRole("button", { name: "Add filter" }));
    await user.click(screen.getByRole("button", { name: "Owner" }));
    await user.click(screen.getByRole("radio", { name: "Unassigned" }));

    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].filters).toEqual({
        unassigned: "true",
      }),
    );
  });

  it("narrow a long owner list by name as the reader types", async () => {
    const user = userEvent.setup();
    render(<Harness fetchPage={stubPage()} initialFilters={{}} />);

    await user.click(await screen.findByRole("button", { name: "Filter" }));
    await user.click(screen.getByRole("button", { name: "Owner" }));
    await user.type(screen.getByPlaceholderText(/Owner/), "anna");

    expect(screen.getByRole("radio", { name: "Anna Becker" })).toBeTruthy();
    expect(screen.queryByRole("radio", { name: "Owned by you" })).toBeNull();
  });
});
