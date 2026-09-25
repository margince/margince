// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import type { ListQuery } from "./listquery";
import { SaveViewAction } from "./savedviews";

// A saved view the reader no longer wants, or whose name no longer says what
// it shows, used to be permanent: a tab can be pressed and nothing else. The
// list's own Save view is joined by a way to rename and delete them.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const UNNARROWED: ListQuery = {
  q: "",
  sort: "",
  includeArchived: false,
  filters: {},
  perPage: 25,
};

const VIEW = {
  id: "v-1",
  owner_id: "u-1",
  resource: "companies",
  name: "German customers",
  version: 3,
  query: { list: { q: "", sort: "", includeArchived: false, filters: {} } },
};

type Seen = {
  method: string;
  path: string;
  ifMatch: string | null;
  body: unknown;
};

// Every request the action makes, and a /views that answers `views` until a
// write has landed and the server's list afterwards.
function stubServer(
  views: unknown[],
  write: (request: Request) => Response = () => Response.json(VIEW),
) {
  const seen: Seen[] = [];
  let after: unknown[] | null = null;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      const body =
        request.method === "PATCH" ? await request.clone().json() : undefined;
      seen.push({
        method: request.method,
        path: pathname,
        ifMatch: request.headers.get("If-Match"),
        body,
      });
      if (pathname.endsWith("/me")) {
        return Response.json(meFixture({}));
      }
      if (request.method === "GET") {
        return Response.json({
          data: after ?? views,
          page: { next_cursor: null, has_more: false },
        });
      }
      const answer = write(request);
      if (answer.ok) {
        after =
          request.method === "DELETE"
            ? []
            : [{ ...VIEW, name: (body as { name: string }).name }];
      }
      return answer;
    }),
  );
  return seen;
}

function draw() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SaveViewAction resource="companies" query={UNNARROWED} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("managing saved views", () => {
  it("offers nothing to manage to a reader with no saved views", async () => {
    const seen = stubServer([]);
    draw();
    await waitFor(() =>
      expect(seen.some((call) => call.path.endsWith("/views"))).toBe(true),
    );
    expect(screen.queryByRole("button", { name: "Manage views" })).toBeNull();
  });

  it("renames a view against the version it was read at", async () => {
    const seen = stubServer([VIEW]);
    const user = userEvent.setup();
    draw();

    await user.click(
      await screen.findByRole("button", { name: "Manage views" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Rename German customers" }),
    );
    const name = screen.getByLabelText("Name");
    expect((name as HTMLInputElement).value).toBe("German customers");
    await user.clear(name);
    await user.type(name, "DACH customers");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("DACH customers")).toBeTruthy();
    expect(seen.find((call) => call.method === "PATCH")).toEqual({
      method: "PATCH",
      path: "/v1/views/v-1",
      ifMatch: "3",
      body: { name: "DACH customers" },
    });
  });

  it("will not save an empty name", async () => {
    stubServer([VIEW]);
    const user = userEvent.setup();
    draw();

    await user.click(
      await screen.findByRole("button", { name: "Manage views" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Rename German customers" }),
    );
    await user.clear(screen.getByLabelText("Name"));

    expect(
      (screen.getByRole("button", { name: "Save" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });

  it("asks before deleting, then removes the view", async () => {
    const seen = stubServer([VIEW]);
    const user = userEvent.setup();
    draw();

    await user.click(
      await screen.findByRole("button", { name: "Manage views" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Delete German customers" }),
    );
    expect(screen.getByText(/removes its tab/)).toBeTruthy();
    expect(seen.some((call) => call.method === "DELETE")).toBe(false);

    await user.click(screen.getByRole("button", { name: "Delete view" }));

    expect(await screen.findByText("No saved views")).toBeTruthy();
    expect(seen.find((call) => call.method === "DELETE")?.path).toBe(
      "/v1/views/v-1",
    );
  });

  it("keeps the row open with the server's reason when a rename is refused", async () => {
    stubServer([VIEW], () =>
      Response.json(
        {
          type: "about:blank",
          title: "Conflict",
          status: 409,
          code: "version_skew",
          detail: "The view changed since it was read. Reload and retry.",
        },
        {
          status: 409,
          headers: { "content-type": "application/problem+json" },
        },
      ),
    );
    const user = userEvent.setup();
    draw();

    await user.click(
      await screen.findByRole("button", { name: "Manage views" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Rename German customers" }),
    );
    await user.type(screen.getByLabelText("Name"), " 2");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByRole("alert")).toBeTruthy();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(
      "German customers 2",
    );
  });
});
