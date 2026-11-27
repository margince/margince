// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
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
import {
  type ListPage,
  type ListQuery,
  ListTable,
  useListQuery,
} from "../../components/listquery";
import {
  type OpenedView,
  OpenedViewContext,
} from "../../components/listqueryviews";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { SAVED_VIEWS_KEY } from "../../lib/savedviews.queries";
import { openView, toggleArchived } from "../../testing/listview.testkit";
import { meFixture } from "../../testing/mefixture";
import { SaveViewAction, useSavedViewTabs } from "./savedviews";
import { ManageViewsButton } from "./savedviews.manage";

// What the View panel offers once the reader has views of this list. Nothing
// while a view already holds the list on screen. An update of the view they
// opened and changed. Never a second view under a name they already use.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const BY_NAME: ListQuery = {
  q: "",
  sort: "full_name",
  includeArchived: false,
  filters: { tag_id: "t-vip" },
  perPage: 25,
};

const VIP = {
  id: "v-1",
  owner_id: "u-1",
  resource: "contacts",
  name: "VIP by name",
  version: 4,
  query: {
    // A key another surface stores beside the list state.
    columns: ["full_name"],
    list: {
      q: "",
      sort: "full_name",
      includeArchived: false,
      filters: { tag_id: "t-vip" },
      perPage: 25,
    },
  },
};

type Write = Readonly<{
  method: string;
  path: string;
  ifMatch: string | null;
  body: unknown;
}>;

// A /views that answers `views`, and records each write it is sent.
function stubViews(
  views: unknown[],
  refuseWrites: boolean | "view_name_taken" = false,
) {
  const writes: Write[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "GET") {
        return Response.json({
          data: views,
          page: { next_cursor: null, has_more: false },
        });
      }
      writes.push({
        method: request.method,
        path: new URL(request.url).pathname,
        ifMatch: request.headers.get("If-Match"),
        body: await request.clone().json(),
      });
      if (refuseWrites === "view_name_taken") {
        return Response.json(
          { code: "view_name_taken", detail: "The name is taken." },
          { status: 409 },
        );
      }
      if (refuseWrites) {
        return Response.json(
          { code: "version_skew", detail: "The view changed." },
          { status: 409 },
        );
      }
      return Response.json(VIP);
    }),
  );
  return writes;
}

function wrap(ui: ReactNode, opened: OpenedView | null = null) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <OpenedViewContext.Provider value={opened}>
            {ui}
          </OpenedViewContext.Provider>
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const saveView = () => screen.queryByRole("button", { name: en["views.save"] });
const saveAsNew = () =>
  screen.queryByRole("button", { name: en["filters.saveAsNew"] });
const update = () =>
  screen.queryByRole("button", { name: "Update “VIP by name”" });

describe("Save view beside the reader's saved views", () => {
  it("is not offered while a saved view already holds the list on screen", async () => {
    stubViews([VIP]);
    wrap(<SaveViewAction resource="contacts" state={{ query: BY_NAME }} />);

    // Manage views arrives with the read of the views.
    await screen.findByRole("button", { name: en["views.manage"] });
    expect(saveView()).toBeNull();
    expect(saveAsNew()).toBeNull();
    expect(update()).toBeNull();
  });

  it("does not count a view that opens in the builder as holding the list, even when its list state matches", async () => {
    const built = {
      ...VIP,
      query: {
        ...VIP.query,
        filter: { and: [{ field: "city", op: "eq", value: "Berlin" }] },
      },
    };
    stubViews([built]);
    function Tabs() {
      const tabs = useSavedViewTabs("contacts");
      return <p>{tabs.map((tab) => tab.link?.name ?? tab.label).join("|")}</p>;
    }
    wrap(
      <>
        <SaveViewAction resource="contacts" state={{ query: BY_NAME }} />
        <Tabs />
      </>,
      { id: built.id, version: built.version },
    );

    expect(await screen.findByText(en["views.save"])).toBeTruthy();
    expect(update()).toBeNull();
    expect(
      screen.getByText("VIP by name, opens in Filters and views"),
    ).toBeTruthy();
  });

  it("treats another page size as the same list", async () => {
    stubViews([VIP]);
    wrap(
      <SaveViewAction
        resource="contacts"
        state={{ query: { ...BY_NAME, perPage: 100 } }}
      />,
    );

    await screen.findByRole("button", { name: en["views.manage"] });
    expect(saveView()).toBeNull();
  });

  it("offers only a new view when the reader opened none of them", async () => {
    stubViews([VIP]);
    wrap(
      <SaveViewAction
        resource="contacts"
        state={{ query: { ...BY_NAME, sort: "-created_at" } }}
      />,
    );

    await screen.findByRole("button", { name: en["views.manage"] });
    expect(saveView()).toBeTruthy();
    expect(update()).toBeNull();
  });

  it("puts a stale-list server name conflict on the name field", async () => {
    const user = userEvent.setup();
    const writes = stubViews([VIP], "view_name_taken");
    wrap(
      <SaveViewAction
        resource="contacts"
        state={{ query: { ...BY_NAME, sort: "-created_at" } }}
      />,
    );
    await screen.findByRole("button", { name: en["views.manage"] });
    await user.click(screen.getByRole("button", { name: en["views.save"] }));
    const name = screen.getByRole("textbox", { name: en["views.name"] });
    await user.type(name, "New in another tab");
    await user.click(
      screen.getByRole("button", { name: en["views.saveConfirm"] }),
    );
    expect(await screen.findByText(en["views.nameTaken"])).toBeTruthy();
    expect(name.getAttribute("aria-invalid")).toBe("true");
    expect(writes).toHaveLength(1);
    await user.type(name, " revised");
    expect(screen.queryByText(en["views.nameTaken"])).toBeNull();
  });

  it("refuses a name one of the reader's views already has, whatever its case", async () => {
    const user = userEvent.setup();
    const writes = stubViews([VIP]);
    wrap(
      <SaveViewAction
        resource="contacts"
        state={{ query: { ...BY_NAME, sort: "-created_at" } }}
      />,
    );

    await screen.findByRole("button", { name: en["views.manage"] });
    await user.click(screen.getByRole("button", { name: en["views.save"] }));
    const name = screen.getByRole("textbox", { name: en["views.name"] });
    await user.type(name, " vip BY name ");
    await user.click(
      screen.getByRole("button", { name: en["views.saveConfirm"] }),
    );

    expect(await screen.findByText(en["views.nameTaken"])).toBeTruthy();
    expect(writes).toEqual([]);

    await user.clear(name);
    await user.type(name, "VIP newest first");
    await user.click(
      screen.getByRole("button", { name: en["views.saveConfirm"] }),
    );
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toMatchObject({
      method: "POST",
      body: { name: "VIP newest first" },
    });
  });

  // A name checked against views not yet read would pass any name.
  it("is not offered until the reader's views are read", async () => {
    let answer: (views: unknown[]) => void = () => undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            answer = (views) =>
              resolve(
                Response.json({
                  data: views,
                  page: { next_cursor: null, has_more: false },
                }),
              );
          }),
      ),
    );
    wrap(
      <SaveViewAction
        resource="contacts"
        state={{ query: { ...BY_NAME, sort: "-created_at" } }}
      />,
    );

    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(saveView()).toBeNull();

    answer([VIP]);
    expect(
      await screen.findByRole("button", { name: en["views.save"] }),
    ).toBeTruthy();
  });

  it("confirms a saved view with a message", async () => {
    const user = userEvent.setup();
    stubViews([]);
    wrap(<SaveViewAction resource="contacts" state={{ query: BY_NAME }} />);

    await user.click(
      await screen.findByRole("button", { name: en["views.save"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["views.name"] }),
      "VIP by name",
    );
    await user.click(
      screen.getByRole("button", { name: en["views.saveConfirm"] }),
    );

    expect(await screen.findByText(en["filters.viewSaved"])).toBeTruthy();
  });
});

describe("a saved view the reader opened and then changed", () => {
  const changed = { ...BY_NAME, sort: "-created_at" };

  it("is offered an update beside a save as a new view", async () => {
    stubViews([VIP]);
    wrap(<SaveViewAction resource="contacts" state={{ query: changed }} />, {
      id: VIP.id,
      version: VIP.version,
    });

    expect(await screen.findByRole("button", { name: /^Update/ })).toBe(
      update(),
    );
    expect(saveAsNew()).toBeTruthy();
    expect(saveView()).toBeNull();
  });

  it("writes the list on screen over that view, at the version it was read", async () => {
    const user = userEvent.setup();
    const writes = stubViews([VIP]);
    wrap(<SaveViewAction resource="contacts" state={{ query: changed }} />, {
      id: VIP.id,
      version: VIP.version,
    });

    await user.click(await screen.findByRole("button", { name: /^Update/ }));

    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toEqual({
      method: "PATCH",
      path: "/v1/views/v-1",
      ifMatch: "4",
      body: {
        query: {
          columns: ["full_name"],
          list: {
            q: "",
            sort: "-created_at",
            includeArchived: false,
            filters: { tag_id: "t-vip" },
            perPage: 25,
          },
        },
      },
    });
    expect(await screen.findByText("View updated: “VIP by name”")).toBeTruthy();
  });

  it("says why when the update is refused", async () => {
    const user = userEvent.setup();
    stubViews([VIP], true);
    wrap(<SaveViewAction resource="contacts" state={{ query: changed }} />, {
      id: VIP.id,
      version: VIP.version,
    });

    await user.click(await screen.findByRole("button", { name: /^Update/ }));

    expect(await screen.findByText(en["common.versionSkew"])).toBeTruthy();
    expect(screen.queryByText(/^View updated/)).toBeNull();
  });

  it("offers no update for an opened tab that is not one of the saved views", async () => {
    stubViews([VIP]);
    wrap(<SaveViewAction resource="contacts" state={{ query: changed }} />, {
      id: "preset:list.viewMine",
    });

    await screen.findByRole("button", { name: en["views.manage"] });
    expect(update()).toBeNull();
    expect(saveView()).toBeTruthy();
  });
});

describe("renaming a saved view", () => {
  it("refuses the name of another of the reader's views", async () => {
    const user = userEvent.setup();
    const other = { ...VIP, id: "v-2", name: "Newest first" };
    const writes = stubViews([VIP, other]);
    wrap(<ManageViewsButton views={[VIP, other]} />);

    await user.click(screen.getByRole("button", { name: en["views.manage"] }));
    await user.click(
      screen.getByRole("button", { name: "Rename Newest first" }),
    );
    const name = screen.getByRole("textbox", { name: en["views.name"] });
    await user.clear(name);
    await user.type(name, "VIP by name");
    await user.click(
      screen.getByRole("button", { name: en["views.saveConfirm"] }),
    );

    expect(await screen.findByText(en["views.nameTaken"])).toBeTruthy();
    expect(writes).toEqual([]);
  });
});

describe("a saved view another writer changed after the reader opened it", () => {
  type Row = { id: string; name: string };
  const emptyPage = async (): Promise<ListPage<Row>> => ({
    data: [],
    page: { next_cursor: null, has_more: false },
  });
  const BY_NAME_VIEW = {
    ...VIP,
    query: {
      list: {
        q: "",
        sort: "name",
        includeArchived: false,
        filters: {},
        perPage: 25,
      },
    },
  };

  function Contacts() {
    const state = useListQuery<Row>({
      key: "views-update",
      fetchPage: emptyPage,
    });
    return (
      <ListTable
        state={state}
        unit="nav.contacts"
        columns={[
          {
            key: "name",
            header: "contacts.name",
            cell: (row: Row) => row.name,
          },
        ]}
        rowKey={(row) => row.id}
        views={[{ label: "list.viewAll" }]}
        dataViews={useSavedViewTabs("contacts")}
        saveView={<SaveViewAction resource="contacts" state={state} />}
      />
    );
  }

  // The server's /views: it holds one row, and refuses a write held to an
  // older version than the one it holds, as If-Match does.
  function stubServer() {
    const server = { view: BY_NAME_VIEW, ifMatch: [] as (string | null)[] };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.method === "GET") {
          return Response.json({
            data: [server.view],
            page: { next_cursor: null, has_more: false },
          });
        }
        const ifMatch = request.headers.get("If-Match");
        server.ifMatch.push(ifMatch);
        if (ifMatch !== String(server.view.version)) {
          return Response.json(
            {
              code: "version_skew",
              detail: "The view changed.",
            },
            { status: 409 },
          );
        }
        const body = (await request.json()) as { query: typeof VIP.query };
        server.view = {
          ...server.view,
          query: { list: { ...server.view.query.list, ...body.query.list } },
          version: server.view.version + 1,
        };
        return Response.json(server.view);
      }),
    );
    return server;
  }

  function mount() {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(["me"], meFixture({ rowScope: "all" }));
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ToastProvider>
            <Contacts />
            <ToastRegion />
          </ToastProvider>
        </LocaleProvider>
      </QueryClientProvider>,
    );
    return client;
  }

  it("holds the update to the version opened, though a refetch brought a newer one", async () => {
    const user = userEvent.setup();
    const server = stubServer();
    const client = mount();

    await user.click(await screen.findByRole("button", { name: VIP.name }));
    await toggleArchived(user);
    // Another writer renames the view; the rail reads it again.
    server.view = { ...BY_NAME_VIEW, name: "VIP, renamed", version: 5 };
    await client.invalidateQueries({ queryKey: [SAVED_VIEWS_KEY] });

    const panel = await openView(user);
    await user.click(
      await within(panel).findByRole("button", {
        name: "Update “VIP, renamed”",
      }),
    );

    expect(await screen.findByText(en["common.versionSkew"])).toBeTruthy();
    expect(server.ifMatch).toEqual(["4"]);
    expect(screen.queryByText(/^View updated/)).toBeNull();
  });

  // The reader's own update is the version they now hold. Once the list shows
  // the view again, a second change and update is held to that version.
  it("holds a second update to the version the reader's own update wrote", async () => {
    const user = userEvent.setup();
    const server = stubServer();
    mount();

    await user.click(await screen.findByRole("button", { name: VIP.name }));
    await toggleArchived(user);
    await user.click(
      await within(await openView(user)).findByRole("button", {
        name: /^Update/,
      }),
    );
    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: VIP.name })
          .getAttribute("aria-pressed"),
      ).toBe("true"),
    );
    await toggleArchived(user);
    await user.click(
      await within(await openView(user)).findByRole("button", {
        name: /^Update/,
      }),
    );

    await waitFor(() => expect(server.ifMatch).toEqual(["4", "5"]));
    expect(screen.queryByText(en["common.versionSkew"])).toBeNull();
  });
});
