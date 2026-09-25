/** @vitest-environment happy-dom */
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
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { stubClipboard } from "../design-system/clipboard-testing";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { ForecastShareActions, ShareViewButton } from "./analytics.share";
import { OPEN_SHARES_KEY } from "./analytics.sharelist";

// The share dialog's two obligations to a reader.
//
// One: the two kinds are told apart in WORDS, because a reader handed a frozen
// number without being told it is frozen reads a three-week-old figure as
// current. Two: the link is shown once and the dialog says what leaving costs,
// because nothing can read it back.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// `staleTime` Infinity makes a cached list stay put on remount, so a test can
// tell a list that was invalidated from one that was merely mounted again.
const render = (ui: ReactNode, staleTime = 0) => {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime },
      mutations: { retry: false },
    },
  });
  const result = rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...result, client };
};

function shareStub(token = "tok-abc") {
  return vi.fn(
    async () =>
      new Response(
        JSON.stringify({
          id: "share-1",
          kind: "live",
          target: "forecast",
          expires_at: "2026-10-03T00:00:00Z",
          token,
          created_at: "2026-09-03T00:00:00Z",
        }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
  );
}

describe("sharing a forecast view", () => {
  it("distinguishes the live and frozen kinds in words", async () => {
    vi.stubGlobal("fetch", shareStub());
    // A frozen state EXISTS here, which is what makes both kinds offerable.
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
        snapshotId="snap-1"
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));

    // Both kinds named, and each one's promise stated. A label alone leaves a
    // reader guessing which of the two they were handed.
    expect(screen.getByLabelText(/Live view/)).toBeTruthy();
    expect(screen.getByText(/Recalculated on each open/)).toBeTruthy();
    expect(
      screen.getByText(/as they stood when the snapshot was taken/),
    ).toBeTruthy();
  });

  it("says the frozen kind is unavailable when nothing has been frozen", async () => {
    vi.stubGlobal("fetch", shareStub());
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));

    // Offered and then refused by the server is the shape to avoid: the reader
    // presses a choice, waits, and is told no.
    expect(
      screen.getByText("No snapshot exists for this period yet."),
    ).toBeTruthy();
  });

  it("shows the link once and says what leaving costs", async () => {
    vi.stubGlobal("fetch", shareStub("tok-xyz"));
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));
    await userEvent.click(screen.getByRole("button", { name: "Create link" }));

    const link = await screen.findByTestId("forecast-share-link");
    expect(link.textContent).toContain("tok-xyz");
    expect(screen.getByText(/link is shown only once/)).toBeTruthy();
    expect(
      screen.getByText(/Leaving without copying discards the link/),
    ).toBeTruthy();
  });

  it("tells the reader to copy by hand when the clipboard refuses", async () => {
    vi.stubGlobal("fetch", shareStub());
    // No clipboard at all — an http origin, which is where this actually
    // happens. Silently doing nothing would leave the reader pressing Copy.
    stubClipboard("absent");
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Share view" }));
    await userEvent.click(screen.getByRole("button", { name: "Create link" }));
    await userEvent.click(
      await screen.findByRole("button", { name: "Copy link" }),
    );

    expect(await screen.findByText(/clipboard access denied/i)).toBeTruthy();
    expect(screen.getByText(/copy it manually/i)).toBeTruthy();
  });

  it("closes the link it just issued, before its expiry", async () => {
    const issue = shareStub();
    const calls: Array<{ method: string; path: string }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        calls.push({
          method: request.method,
          path: new URL(request.url).pathname,
        });
        return request.method === "DELETE"
          ? new Response(null, { status: 204 })
          : issue();
      }),
    );
    const user = userEvent.setup();
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Share view" }));
    await user.click(screen.getByRole("button", { name: "Create link" }));
    await user.click(await screen.findByRole("button", { name: "Close link" }));

    // The dialog stops offering a link that no longer opens.
    expect(await screen.findByText("Link closed")).toBeTruthy();
    expect(screen.queryByTestId("forecast-share-link")).toBeNull();
    expect(calls).toContainEqual({
      method: "DELETE",
      path: "/v1/forecast/shares/share-1",
    });
  });

  it("keeps the link on screen with the reason when closing is refused", async () => {
    const issue = shareStub("tok-kept");
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) =>
        request.method === "DELETE"
          ? new Response(
              JSON.stringify({
                title: "Forbidden",
                status: 403,
                detail: "Only the colleague who issued a share can close it.",
              }),
              {
                status: 403,
                headers: { "Content-Type": "application/problem+json" },
              },
            )
          : issue(),
      ),
    );
    const user = userEvent.setup();
    render(
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole company" }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Share view" }));
    await user.click(screen.getByRole("button", { name: "Create link" }));
    await user.click(await screen.findByRole("button", { name: "Close link" }));

    expect(
      await screen.findByText(/Only the colleague who issued a share/),
    ).toBeTruthy();
    expect(screen.getByTestId("forecast-share-link").textContent).toContain(
      "tok-kept",
    );
  });
});

// The list of a reader's open links, behind Shared links beside Share view.
//
// A server stand-in keyed "METHOD /path" that records every request, so a case
// can say what was asked for as well as what was drawn.
type Route = (request: Request) => Response | Promise<Response>;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const TEAM_NORTH = "5f0d7a1e-8c2b-4d3a-9e61-0a4b2c9d7e11";
const GONE_TEAM = "9c1a3e5f-7b2d-4f60-8a4e-3d2c1b0a9f31";

type OpenShare = components["schemas"]["ForecastShare"];

const northLink: OpenShare = {
  id: "share-north",
  kind: "live",
  target: "forecast",
  scope_kind: "team",
  scope_id: TEAM_NORTH,
  created_at: "2026-09-12T09:00:00Z",
  expires_at: "2026-10-12T09:00:00Z",
};
const companyLink: OpenShare = {
  id: "share-company",
  kind: "snapshot",
  target: "forecast",
  scope_kind: "workspace",
  created_at: "2026-09-08T09:00:00Z",
  expires_at: "2026-10-08T09:00:00Z",
};
const goneLink: OpenShare = {
  id: "share-gone",
  kind: "live",
  target: "forecast",
  scope_kind: "team",
  scope_id: GONE_TEAM,
  created_at: "2026-09-03T09:00:00Z",
  expires_at: "2026-10-03T09:00:00Z",
};

function serve(
  routes: Record<string, Route>,
  allow: GrantSpec,
  seat: "full" | "read" = "full",
) {
  const calls: string[] = [];
  const all: Record<string, Route> = {
    "GET /me": () => json(meFixture({ roles: ["manager"], allow, seat })),
    "GET /analytics/context": () =>
      json({
        default_scope: { kind: "workspace", label: "Whole company" },
        allowed_scopes: [
          { kind: "workspace", label: "Whole company" },
          { kind: "team", id: TEAM_NORTH, label: "Team North" },
        ],
        capabilities: {
          view_manager_forecast: true,
          submit_manager_forecast: true,
        },
      }),
    ...routes,
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      const key = `${request.method} ${path}`;
      calls.push(key);
      const route = all[key];
      return route ? route(request) : json({ title: "Not Found" }, 404);
    }),
  );
  return calls;
}

const FORECAST_CREATE: GrantSpec = { forecast: ["create"] };

function mountActions(staleTime = 0) {
  return render(
    <ForecastShareActions
      target="forecast"
      scope={{ kind: "workspace", label: "Whole company" }}
    />,
    staleTime,
  );
}

function rowOf(population: string): HTMLElement {
  const row = screen.getByText(population).closest(".panel-row");
  if (!(row instanceof HTMLElement)) {
    throw new Error(`no row names ${population}`);
  }
  return row;
}

describe("the links a reader has shared", () => {
  it("shows neither verb and asks for no list to a seat without forecast:create", async () => {
    const calls = serve({}, { forecast: ["read"] });
    mountActions();

    // Settled once /me has answered: the gate reads the grant, not its absence.
    await waitFor(() => expect(calls).toContain("GET /me"));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Share view" })).toBeNull(),
    );
    expect(screen.queryByRole("button", { name: "Shared links" })).toBeNull();
    expect(calls).not.toContain("GET /forecast/shares");
  });

  it("lists each open link with its kind, population and dates", async () => {
    serve(
      {
        "GET /forecast/shares": () =>
          json({ data: [northLink, companyLink, goneLink] }),
      },
      FORECAST_CREATE,
    );
    const user = userEvent.setup();
    mountActions();

    await user.click(
      await screen.findByRole("button", { name: "Shared links" }),
    );
    const drawer = await screen.findByRole("dialog", {
      name: "Your shared links",
    });

    const north = await within(drawer).findByText("Team North");
    expect(north).toBeTruthy();
    const zone = viewerZone();
    const northRow = rowOf("Team North");
    expect(northRow.textContent).toContain("Live view");
    expect(northRow.textContent).toContain(
      `Created ${formatDate(northLink.created_at, "en", zone)}`,
    );
    expect(northRow.textContent).toContain(
      `Expires ${formatDate(northLink.expires_at, "en", zone)}`,
    );
    expect(rowOf("Whole company").textContent).toContain("Snapshot");
    // A team the picker no longer offers is named by its kind, never its id.
    expect(within(drawer).getByText("Team")).toBeTruthy();
    expect(drawer.textContent).not.toContain(GONE_TEAM);
  });

  it("says so when no link is open", async () => {
    serve(
      { "GET /forecast/shares": () => json({ data: [] }) },
      FORECAST_CREATE,
    );
    const user = userEvent.setup();
    mountActions();

    await user.click(
      await screen.findByRole("button", { name: "Shared links" }),
    );

    expect(await screen.findByText(/You have no open links/)).toBeTruthy();
  });

  it("closes the chosen link after asking, and keeps the drawer open", async () => {
    let open = [northLink, companyLink];
    const calls = serve(
      {
        "GET /forecast/shares": () => json({ data: open }),
        "DELETE /forecast/shares/share-north": () => {
          open = [companyLink];
          return new Response(null, { status: 204 });
        },
      },
      FORECAST_CREATE,
    );
    const user = userEvent.setup();
    mountActions();

    await user.click(
      await screen.findByRole("button", { name: "Shared links" }),
    );
    await user.click(
      await screen.findByRole("button", {
        name: "Close link",
        description: "Team North",
      }),
    );
    // Asked first: nothing is closed until the reader confirms.
    const confirm = await screen.findByRole("dialog", {
      name: "Close this link?",
    });
    expect(calls).not.toContain("DELETE /forecast/shares/share-north");
    await user.click(
      within(confirm).getByRole("button", { name: "Close link" }),
    );

    await waitFor(() => expect(screen.queryByText("Team North")).toBeNull());
    expect(calls).toContain("DELETE /forecast/shares/share-north");
    expect(
      screen.getByRole("dialog", { name: "Your shared links" }),
    ).toBeTruthy();
    expect(screen.getByText("Whole company")).toBeTruthy();
  });

  it("keeps the confirmation open with the reason when closing is refused", async () => {
    serve(
      {
        "GET /forecast/shares": () => json({ data: [northLink] }),
        "DELETE /forecast/shares/share-north": () =>
          new Response(
            JSON.stringify({
              title: "Not Found",
              status: 404,
              detail: "No share of yours has that id.",
            }),
            {
              status: 404,
              headers: { "Content-Type": "application/problem+json" },
            },
          ),
      },
      FORECAST_CREATE,
    );
    const user = userEvent.setup();
    mountActions();

    await user.click(
      await screen.findByRole("button", { name: "Shared links" }),
    );
    await user.click(await screen.findByRole("button", { name: "Close link" }));
    const confirm = await screen.findByRole("dialog", {
      name: "Close this link?",
    });
    await user.click(
      within(confirm).getByRole("button", { name: "Close link" }),
    );

    expect(
      await within(confirm).findByText(/No share of yours has that id/),
    ).toBeTruthy();
    expect(screen.getByText("Team North")).toBeTruthy();
  });

  it("re-reads the list after a share is issued", async () => {
    let open: unknown[] = [];
    const calls = serve(
      {
        "GET /forecast/shares": () => json({ data: open }),
        "POST /forecast/shares": () => {
          open = [companyLink];
          return json(
            {
              ...companyLink,
              kind: "live",
              token: "tok-new",
            },
            201,
          );
        },
      },
      FORECAST_CREATE,
    );
    const user = userEvent.setup();
    mountActions(Number.POSITIVE_INFINITY);

    await user.click(
      await screen.findByRole("button", { name: "Shared links" }),
    );
    await screen.findByText(/You have no open links/);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(screen.getByRole("button", { name: "Share view" }));
    await user.click(screen.getByRole("button", { name: "Create link" }));
    await user.click(await screen.findByRole("button", { name: "Done" }));
    await user.click(screen.getByRole("button", { name: "Shared links" }));

    expect(await screen.findByText("Whole company")).toBeTruthy();
    expect(
      calls.filter((call) => call === "GET /forecast/shares"),
    ).toHaveLength(2);
  });

  it("re-reads the list after a link is closed from the dialog that issued it", async () => {
    let open = [northLink, companyLink];
    const calls = serve(
      {
        "GET /forecast/shares": () => json({ data: open }),
        "POST /forecast/shares": () =>
          json({ ...northLink, token: "tok-north" }, 201),
        "DELETE /forecast/shares/share-north": () => {
          open = [companyLink];
          return new Response(null, { status: 204 });
        },
      },
      FORECAST_CREATE,
    );
    const user = userEvent.setup();
    const { client } = mountActions(Number.POSITIVE_INFINITY);

    await user.click(await screen.findByRole("button", { name: "Share view" }));
    await user.click(screen.getByRole("button", { name: "Create link" }));
    await screen.findByTestId("forecast-share-link");
    // The list as it stood once the link was issued, and fresh: from here only
    // the close can make it stale.
    client.setQueryData(OPEN_SHARES_KEY, [northLink, companyLink]);
    await user.click(screen.getByRole("button", { name: "Close link" }));
    await user.click(await screen.findByRole("button", { name: "Done" }));
    await user.click(screen.getByRole("button", { name: "Shared links" }));

    expect(await screen.findByText("Whole company")).toBeTruthy();
    await waitFor(() => expect(screen.queryByText("Team North")).toBeNull());
    expect(
      calls.filter((call) => call === "GET /forecast/shares"),
    ).toHaveLength(1);
  });

  it("lists the links to a read seat without offering to close them", async () => {
    const calls = serve(
      {
        "GET /forecast/shares": () => json({ data: [northLink, companyLink] }),
      },
      FORECAST_CREATE,
      "read",
    );
    const user = userEvent.setup();
    mountActions();

    await user.click(
      await screen.findByRole("button", { name: "Shared links" }),
    );

    expect(await screen.findByText("Team North")).toBeTruthy();
    expect(screen.getByText("Whole company")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Close link" })).toBeNull();
    // Issuing is a write too, so the read seat is not offered it either.
    expect(screen.queryByRole("button", { name: "Share view" })).toBeNull();
    expect(calls).toContain("GET /forecast/shares");
  });

  it("hands focus to the link now in a closed middle link's place", async () => {
    const { user, closeVerb, confirmClose } =
      await closeOneOfThree(companyLink);

    await user.click(closeVerb("Whole company"));
    await confirmClose();

    // The link below moves up into the closed one's place.
    await waitFor(() => expect(document.activeElement).toBe(closeVerb("Team")));
  });

  it("hands focus to the link above when the last link is closed", async () => {
    const { user, closeVerb, confirmClose } = await closeOneOfThree(goneLink);

    await user.click(closeVerb("Team"));
    await confirmClose();

    await waitFor(() =>
      expect(document.activeElement).toBe(closeVerb("Whole company")),
    );
  });
});

// Three open links, the drawer open on them, and one of them closable: the
// server drops `closed` from the list once its DELETE arrives.
async function closeOneOfThree(closed: OpenShare) {
  let open = [northLink, companyLink, goneLink];
  serve(
    {
      "GET /forecast/shares": () => json({ data: open }),
      [`DELETE /forecast/shares/${closed.id}`]: () => {
        open = open.filter((share) => share.id !== closed.id);
        return new Response(null, { status: 204 });
      },
    },
    FORECAST_CREATE,
  );
  const user = userEvent.setup();
  mountActions();
  await user.click(await screen.findByRole("button", { name: "Shared links" }));
  await screen.findByText("Whole company");
  const closeVerb = (population: string) =>
    screen.getByRole("button", { name: "Close link", description: population });
  const confirmClose = async () => {
    const confirm = screen.getByRole("dialog", { name: "Close this link?" });
    await user.click(
      within(confirm).getByRole("button", { name: "Close link" }),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Close this link?" }),
      ).toBeNull(),
    );
  };
  return { user, closeVerb, confirmClose };
}
