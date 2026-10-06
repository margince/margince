/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useRef, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { type SavedView, useSavedViews } from "./savedviews.queries";
import { DeleteViewAction, RenameViewAction } from "./viewactions";

// One saved view's Rename and Delete, each its own dialog, served to a caller
// whose view can change or vanish under it while the dialog is open.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const VIEW: SavedView = {
  id: "v1",
  owner_id: "u-1",
  shared_scope: "private",
  resource: "contacts",
  name: "Berlin contacts",
  query: { filter: { and: [{ field: "city", op: "eq", value: "Berlin" }] } },
  version: 3,
};

type Patch = Readonly<{ ifMatch: string | null; body: unknown }>;

/**
 * A /views holding `stored`, refusing a rename held to any version but the one
 * it has, and forgetting a view once it is deleted.
 */
function stubViews(stored: SavedView) {
  const patches: Patch[] = [];
  let views = [stored];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "DELETE") {
        views = [];
        return new Response(null, { status: 204 });
      }
      if (request.method === "PATCH") {
        const ifMatch = request.headers.get("If-Match");
        patches.push({ ifMatch, body: await request.json() });
        return ifMatch === String(stored.version)
          ? Response.json(stored)
          : Response.json(
              {
                title: "Conflict",
                status: 409,
                code: "version_skew",
                detail: "The view changed since it was read.",
              },
              {
                status: 409,
                headers: { "Content-Type": "application/problem+json" },
              },
            );
      }
      return Response.json({
        data: views,
        page: { next_cursor: null, has_more: false },
      });
    }),
  );
  return patches;
}

function Providers({ children }: Readonly<{ children: ReactNode }>) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      }),
  );
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

/** A caller whose row goes with the view, naming the list as focus's landing. */
function ViewRows() {
  const views = useSavedViews("contacts");
  const list = useRef<HTMLUListElement>(null);
  return (
    <ul ref={list} tabIndex={-1} aria-label="Views">
      {views.data?.map((view) => (
        <li key={view.id}>
          {view.name}
          <DeleteViewAction view={view} returnFocusTo={() => list.current} />
        </li>
      ))}
    </ul>
  );
}

describe("renaming a view", () => {
  it("holds the name to the version the dialog opened on, though a newer one was read since", async () => {
    const renamedElsewhere = { ...VIEW, name: "Berliners", version: 4 };
    const patches = stubViews(renamedElsewhere);
    const user = userEvent.setup();
    const { rerender } = render(<RenameViewAction view={VIEW} />, {
      wrapper: Providers,
    });
    await user.click(screen.getByRole("button", { name: en["views.rename"] }));
    rerender(<RenameViewAction view={renamedElsewhere} />);
    const name = screen.getByRole("textbox", { name: en["views.name"] });
    await user.clear(name);
    await user.type(name, "Berlin leads");
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["views.rename"],
      }),
    );

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(patches).toEqual([{ ifMatch: "3", body: { name: "Berlin leads" } }]);
  });
});

describe("deleting a view", () => {
  it("hands focus to where its caller says once the row has gone with the view", async () => {
    stubViews(VIEW);
    const user = userEvent.setup();
    render(<ViewRows />, { wrapper: Providers });
    await user.click(
      await screen.findByRole("button", { name: en["views.deleteConfirm"] }),
    );
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["views.deleteConfirm"],
      }),
    );

    await vi.waitFor(() =>
      expect(screen.queryByText("Berlin contacts")).toBeNull(),
    );
    expect(screen.getByRole("list", { name: "Views" })).toHaveFocus();
  });
});
