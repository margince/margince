/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { type ReactNode, useRef, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { type SavedView, useSavedViews } from "../../lib/savedviews.queries";
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
  resource: "contacts",
  name: "Berlin contacts",
  query: { filter: { and: [{ field: "city", op: "eq", value: "Berlin" }] } },
  version: 3,
};

type Patch = Readonly<{ ifMatch: string | null; body: unknown }>;

/**
 * A /views holding `stored`: a rename held to the version it has lands and
 * moves it on, one held to any other is refused, and a delete forgets it.
 */
function stubViews(stored: SavedView, beside: readonly SavedView[] = []) {
  const patches: Patch[] = [];
  let views = [stored, ...beside];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "DELETE") {
        views = [];
        return new Response(null, { status: 204 });
      }
      if (request.method === "PATCH") {
        const ifMatch = request.headers.get("If-Match");
        const body: { name: string } = await request.json();
        patches.push({ ifMatch, body });
        const held = views.find((view) => String(view.version) === ifMatch);
        if (held) {
          const renamed = { ...held, ...body, version: held.version + 1 };
          views = [renamed];
          return Response.json(renamed);
        }
        return Response.json(
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
    const { rerender } = render(
      <RenameViewAction view={VIEW} resource="contacts" />,
      {
        wrapper: Providers,
      },
    );
    await user.click(screen.getByRole("button", { name: en["views.rename"] }));
    rerender(<RenameViewAction view={renamedElsewhere} resource="contacts" />);
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

  it("closes on the server's answer, says so, and hands that answer on", async () => {
    stubViews(VIEW);
    const renamed = vi.fn();
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <RenameViewAction view={VIEW} resource="contacts" onRenamed={renamed} />
        <ToastRegion />
      </ToastProvider>,
      { wrapper: Providers },
    );
    await user.click(screen.getByRole("button", { name: en["views.rename"] }));
    const name = screen.getByRole("textbox", { name: en["views.name"] });
    await user.clear(name);
    await user.type(name, "Berlin leads");
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["views.rename"],
      }),
    );

    expect(await screen.findByText(en["views.renamed"])).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(renamed).toHaveBeenCalledExactlyOnceWith({
      ...VIEW,
      name: "Berlin leads",
      version: 4,
    });
  });

  it("refuses a name another view of the resource holds, as Manage views does", async () => {
    const patches = stubViews(VIEW, [
      { ...VIEW, id: "v2", name: "Munich contacts", version: 1 },
    ]);
    const user = userEvent.setup();
    render(<RenameViewAction view={VIEW} resource="contacts" />, {
      wrapper: Providers,
    });
    await user.click(screen.getByRole("button", { name: en["views.rename"] }));
    const name = screen.getByRole("textbox", { name: en["views.name"] });
    await user.clear(name);
    await user.type(name, " munich CONTACTS ");
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["views.rename"],
      }),
    );

    expect(await screen.findByText(en["views.nameTaken"])).toBeInTheDocument();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(patches).toEqual([]);
  });
});

describe("renaming a view after a taken name", () => {
  it("saves a free name typed next, and drops the taken notice", async () => {
    const patches = stubViews(VIEW, [
      { ...VIEW, id: "v2", name: "Munich contacts", version: 1 },
    ]);
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <RenameViewAction view={VIEW} resource="contacts" />
        <ToastRegion />
      </ToastProvider>,
      { wrapper: Providers },
    );
    await user.click(screen.getByRole("button", { name: en["views.rename"] }));
    const name = screen.getByRole("textbox", { name: en["views.name"] });
    const confirm = within(screen.getByRole("dialog")).getByRole("button", {
      name: en["views.rename"],
    });
    await user.clear(name);
    await user.type(name, "Munich contacts");
    await user.click(confirm);
    expect(await screen.findByText(en["views.nameTaken"])).toBeInTheDocument();

    await user.clear(name);
    await user.type(name, "Berlin leads");
    expect(screen.queryByText(en["views.nameTaken"])).toBeNull();
    await user.click(confirm);

    expect(await screen.findByText(en["views.renamed"])).toBeInTheDocument();
    expect(screen.queryByText(en["views.nameTaken"])).toBeNull();
    expect(patches).toEqual([{ ifMatch: "3", body: { name: "Berlin leads" } }]);
  });
});

describe("renaming a view before the reader's views are read", () => {
  const MUNICH = { ...VIEW, id: "v2", name: "Munich contacts", version: 1 };

  /** /views held open until `answer`; every PATCH is kept. */
  function holdViews() {
    const patches: unknown[] = [];
    let answer: (response: Response) => void = () => undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.method === "PATCH") {
          patches.push(await request.json());
          return Response.json({ ...VIEW, version: 4 });
        }
        return new Promise<Response>((resolve) => {
          answer = resolve;
        });
      }),
    );
    return { patches, answer: (response: Response) => answer(response) };
  }

  const refused = () =>
    Response.json(
      { title: "Forbidden", status: 403, detail: "The views read failed." },
      {
        status: 403,
        headers: { "Content-Type": "application/problem+json" },
      },
    );

  const renameTo = async (user: UserEvent, name: string) => {
    const field = screen.getByRole("textbox", { name: en["views.name"] });
    await user.clear(field);
    await user.type(field, `${name}{Enter}`);
  };

  it("saves no name until those views are read, then checks it against them", async () => {
    const held = holdViews();
    const user = userEvent.setup();
    render(<RenameViewAction view={VIEW} resource="contacts" />, {
      wrapper: Providers,
    });
    await user.click(screen.getByRole("button", { name: en["views.rename"] }));
    await renameTo(user, "Munich contacts");
    expect(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["views.rename"],
      }),
    ).toHaveAttribute("aria-busy", "true");
    expect(held.patches).toEqual([]);

    held.answer(
      Response.json({
        data: [VIEW, MUNICH],
        page: { next_cursor: null, has_more: false },
      }),
    );
    await waitFor(() =>
      expect(
        within(screen.getByRole("dialog")).getByRole("button", {
          name: en["views.rename"],
        }),
      ).not.toHaveAttribute("aria-busy", "true"),
    );
    await renameTo(user, "Munich contacts");
    expect(await screen.findByText(en["views.nameTaken"])).toBeInTheDocument();
    expect(held.patches).toEqual([]);
  });

  it("says the views read failed rather than saving a name it could not check", async () => {
    const held = holdViews();
    const user = userEvent.setup();
    render(<RenameViewAction view={VIEW} resource="contacts" />, {
      wrapper: Providers,
    });
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    held.answer(refused());
    await user.click(screen.getByRole("button", { name: en["views.rename"] }));
    // Opening the dialog asks again; that answer is refused too.
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    held.answer(refused());
    await renameTo(user, "Berlin leads");
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The views read failed.",
    );
    expect(held.patches).toEqual([]);
  });

  it("says a failed read again and saves nothing, though an earlier read's names are still held", async () => {
    const held = holdViews();
    function ReadAgain() {
      const views = useSavedViews("contacts");
      return (
        <button type="button" onClick={() => void views.refetch()}>
          Read again
        </button>
      );
    }
    const user = userEvent.setup();
    render(
      <>
        <ReadAgain />
        <RenameViewAction view={VIEW} resource="contacts" />
      </>,
      { wrapper: Providers },
    );
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    held.answer(
      Response.json({
        data: [VIEW],
        page: { next_cursor: null, has_more: false },
      }),
    );
    await user.click(screen.getByRole("button", { name: "Read again" }));
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    held.answer(refused());
    await user.click(screen.getByRole("button", { name: en["views.rename"] }));
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(3));
    held.answer(refused());
    await renameTo(user, "Berlin leads");
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The views read failed.",
    );
    expect(held.patches).toEqual([]);
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
