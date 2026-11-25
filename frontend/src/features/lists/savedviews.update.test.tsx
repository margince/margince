// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ListQuery } from "../../components/listquery";
import { OpenedViewContext } from "../../components/listqueryviews";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { SaveViewAction } from "./savedviews";
import { ManageViewsButton } from "./savedviews.manage";

// What the View panel offers once the reader has views of this list. Nothing
// while a view already holds the list on screen. An update of the view they
// opened and changed. Never a second view under a name they already use.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
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
  shared_scope: "private" as const,
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
function stubViews(views: unknown[], refuseWrites = false) {
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
      return refuseWrites
        ? Response.json(
            { code: "version_skew", detail: "The view changed." },
            { status: 409 },
          )
        : Response.json(VIP);
    }),
  );
  return writes;
}

function wrap(ui: ReactNode, opened: string | null = null) {
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
    wrap(
      <SaveViewAction resource="contacts" state={{ query: changed }} />,
      VIP.id,
    );

    expect(await screen.findByRole("button", { name: /^Update/ })).toBe(
      update(),
    );
    expect(saveAsNew()).toBeTruthy();
    expect(saveView()).toBeNull();
  });

  it("writes the list on screen over that view, at the version it was read", async () => {
    const user = userEvent.setup();
    const writes = stubViews([VIP]);
    wrap(
      <SaveViewAction resource="contacts" state={{ query: changed }} />,
      VIP.id,
    );

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
    wrap(
      <SaveViewAction resource="contacts" state={{ query: changed }} />,
      VIP.id,
    );

    await user.click(await screen.findByRole("button", { name: /^Update/ }));

    expect(await screen.findByText(en["common.versionSkew"])).toBeTruthy();
    expect(screen.queryByText(/^View updated/)).toBeNull();
  });

  it("offers no update for an opened tab that is not one of the saved views", async () => {
    stubViews([VIP]);
    wrap(
      <SaveViewAction resource="contacts" state={{ query: changed }} />,
      "preset:list.viewMine",
    );

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
