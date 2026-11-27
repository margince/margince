/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useContext } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { openView, toggleArchived } from "../testing/listview.testkit";
import { meFixture } from "../testing/mefixture";
import { type ListPage, ListTable, useListQuery } from "./listquery";
import { OpenedViewContext } from "./listqueryviews";

// The View panel's save verbs are told which view the reader opened. They are
// still told once a changed dial has put that view's tab out.

type Row = { id: string; name: string };

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

const emptyPage = async (): Promise<ListPage<Row>> => ({
  data: [],
  page: { next_cursor: null, has_more: false },
});

function Opened() {
  return <output>{useContext(OpenedViewContext)?.id ?? "none"}</output>;
}

function Rail() {
  const state = useListQuery<Row>({ key: "opened-view", fetchPage: emptyPage });
  return (
    <ListTable
      state={state}
      unit="nav.contacts"
      columns={[
        { key: "name", header: "contacts.name", cell: (row: Row) => row.name },
      ]}
      rowKey={(row) => row.id}
      views={[{ label: "list.viewAll" }]}
      dataViews={[
        {
          id: "v-1",
          label: "By name",
          q: "",
          sort: "name",
          filters: {},
          includeArchived: false,
          version: 1,
        },
      ]}
      saveView={<Opened />}
    />
  );
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture({ rowScope: "all" }));
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Rail />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const tab = (name: string) => screen.getByRole("button", { name });
const opened = async (user: ReturnType<typeof userEvent.setup>) =>
  (await openView(user)).querySelector("output")?.textContent;

describe("the view the reader opened", () => {
  it("is the saved view whose tab they pressed", async () => {
    const user = userEvent.setup();
    mount();

    await user.click(await screen.findByRole("button", { name: "By name" }));

    await waitFor(() =>
      expect(tab("By name").getAttribute("aria-pressed")).toBe("true"),
    );
    expect(await opened(user)).toBe("v-1");
  });

  it("stays that view once a changed dial has put its tab out", async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole("button", { name: "By name" }));
    await waitFor(() =>
      expect(tab("By name").getAttribute("aria-pressed")).toBe("true"),
    );

    await toggleArchived(user);

    await waitFor(() =>
      expect(tab("By name").getAttribute("aria-pressed")).toBe("false"),
    );
    expect(await opened(user)).toBe("v-1");
  });

  it("moves to a preset the reader presses next", async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole("button", { name: "By name" }));
    await waitFor(() =>
      expect(tab("By name").getAttribute("aria-pressed")).toBe("true"),
    );

    await user.click(tab("All"));

    await waitFor(() =>
      expect(tab("All").getAttribute("aria-pressed")).toBe("true"),
    );
    expect(await opened(user)).toBe("preset:list.viewAll");
  });
});
