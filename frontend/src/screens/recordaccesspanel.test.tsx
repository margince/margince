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
import { LocaleProvider } from "../i18n";
import {
  REFRESH_CEILING_MS,
  REFRESH_FLOOR_MS,
  WhoCanSeePanel,
} from "./recordaccesspanel";

// The panel groups what the server judged; it decides nothing. These cases
// hold the grouping, the collapsed "everyone" row and the reader's own line.

type RecordAccess = components["schemas"]["RecordAccess"];
type Member = components["schemas"]["RecordAccessMember"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

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

// Serves one answer, or a later page when the request carries its cursor.
function serve(body: RecordAccess, later: Record<string, RecordAccess> = {}) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "https://test.local",
    );
    if (url.pathname.endsWith("/contacts/c-1/access")) {
      const cursor = url.searchParams.get("cursor");
      const page = cursor ? later[cursor] : body;
      return new Response(JSON.stringify(page), {
        headers: { "Content-Type": "application/json" },
      });
    }
    return new Response("{}", { status: 404 });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function member(id: string, name: string, over: Partial<Member>): Member {
  return {
    user_id: id,
    display_name: name,
    group: "everyone",
    can_change: false,
    read_reasons: [{ code: "workspace_visible" }],
    change_reasons: [],
    ...over,
  };
}

const owner = member("u-owner", "Alex Owner", {
  group: "owner",
  can_change: true,
  read_reasons: [{ code: "owner" }],
  change_reasons: [{ code: "owner" }],
});

function answer(over: Partial<RecordAccess>): RecordAccess {
  return {
    visibility: "workspace",
    owner_id: "u-owner",
    archived: false,
    detail: false,
    you: {
      can_change: false,
      read_reasons: [{ code: "workspace_visible" }],
      change_reasons: [],
    },
    data: [
      owner,
      member("u-share", "Priya Shah", {
        group: "shared",
        read_reasons: [{ code: "user_share", access: "read" }],
      }),
      member("u-team", "Mor Adler", {
        group: "team_shared",
        read_reasons: [{ code: "team_share", access: "read" }],
      }),
      member("u-kim", "Kim Rep", {}),
      member("u-noor", "Noor Rep", {}),
    ],
    page: { has_more: false, total: 5 },
    group_counts: { owner: 1, shared: 1, team_shared: 1, everyone: 2 },
    can_change_count: 1,
    team_access_count: 0,
    refresh_at: null,
    ...over,
  };
}

describe("WhoCanSeePanel", () => {
  it("groups colleagues by why they can see the record", async () => {
    serve(answer({}));
    render(<WhoCanSeePanel kind="contact" recordId="c-1" />);

    const ownerGroup = await screen.findByTestId("who-can-see-owner");
    expect(within(ownerGroup).getByText("Alex Owner")).toBeTruthy();
    expect(within(ownerGroup).getByText("Can edit")).toBeTruthy();
    expect(
      within(screen.getByTestId("who-can-see-shared")).getByText("Priya Shah"),
    ).toBeTruthy();
    const team = screen.getByTestId("who-can-see-team_shared");
    expect(within(team).getByText("Mor Adler")).toBeTruthy();
    // A reader outside member administration is not told which team.
    expect(within(team).getByText("Through a team share")).toBeTruthy();
  });

  it("folds everyone else into one closed row that counts them", async () => {
    serve(answer({}));
    render(<WhoCanSeePanel kind="contact" recordId="c-1" />);

    const summary = await screen.findByText("2 users with contact access");
    const details = summary.closest("details");
    expect(details?.open).toBe(false);
    expect(
      within(screen.getByTestId("who-can-see-everyone")).getByText("Kim Rep"),
    ).toBeTruthy();
    expect(screen.queryByTestId("who-can-see-owner")).toBeTruthy();
  });

  it("draws no everyone row on a private record", async () => {
    serve(
      answer({
        visibility: "owner",
        data: [owner],
        page: { has_more: false, total: 1 },
        group_counts: { owner: 1, shared: 0, team_shared: 0, everyone: 0 },
      }),
    );
    render(<WhoCanSeePanel kind="contact" recordId="c-1" />);

    await screen.findByText("Alex Owner");
    expect(screen.queryByText(/with contact access/)).toBeNull();
  });

  it("says the reader's own access on its own line", async () => {
    serve(answer({}));
    render(<WhoCanSeePanel kind="contact" recordId="c-1" />);

    const you = await screen.findByTestId("who-can-see-you");
    expect(you.textContent).toContain(
      "You can view this contact but not edit it.",
    );
    expect(you.textContent).toContain("Open to all users");
  });

  it("names a team and roles only when the server sent them", async () => {
    serve(
      answer({
        detail: true,
        data: [
          { ...owner, roles: ["admin"] },
          member("u-team", "Mor Adler", {
            group: "team_shared",
            read_reasons: [
              { code: "team_share", access: "read", team_name: "Deal Desk" },
            ],
          }),
        ],
        group_counts: { owner: 1, shared: 0, team_shared: 1, everyone: 0 },
      }),
    );
    render(<WhoCanSeePanel kind="contact" recordId="c-1" />);

    expect(await screen.findByText("Through team Deal Desk")).toBeTruthy();
    expect(screen.getByText("Admin")).toBeTruthy();
  });

  it("loads the next page, and a colleague on it joins their group", async () => {
    const user = userEvent.setup();
    serve(
      answer({
        data: [member("u-kim", "Kim Rep", {})],
        page: { has_more: true, next_cursor: "c-2", total: 2 },
        group_counts: { owner: 1, shared: 0, team_shared: 0, everyone: 1 },
      }),
      {
        "c-2": answer({
          data: [owner],
          page: { has_more: false, next_cursor: null, total: 2 },
        }),
      },
    );
    render(<WhoCanSeePanel kind="contact" recordId="c-1" />);

    await screen.findByText("1 user with contact access");
    expect(screen.queryByTestId("who-can-see-owner")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Load more" }));

    const ownerGroup = await screen.findByTestId("who-can-see-owner");
    expect(within(ownerGroup).getByText("Alex Owner")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("counts, without naming, the colleagues a team adds", async () => {
    serve(answer({ team_access_count: 3 }));
    render(<WhoCanSeePanel kind="contact" recordId="c-1" />);

    const line = await screen.findByTestId("who-can-see-team-access");
    expect(line.textContent).toBe(
      "3 more users have access or can edit through a team. Only admins can see who.",
    );
  });

  // Each case runs on fake timers, so the waits cost no wall-clock time.
  async function refetchesAfter(refreshAt: (now: number) => string) {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const fetchMock = serve(answer({ refresh_at: refreshAt(Date.now()) }));
    render(<WhoCanSeePanel kind="contact" recordId="c-1" />);
    await screen.findByTestId("who-can-see-you");
    return fetchMock;
  }

  it("reads the answer again when the earliest share lapses", async () => {
    try {
      const fetchMock = await refetchesAfter((now) =>
        new Date(now + 90_000).toISOString(),
      );
      await vi.advanceTimersByTimeAsync(60_000);
      expect(fetchMock).toHaveBeenCalledTimes(1);
      await vi.advanceTimersByTimeAsync(31_000);
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    } finally {
      vi.useRealTimers();
    }
  });

  // The client clock runs ahead of the server's: the lapse has passed here
  // and not there, so the answer comes back unchanged. It is asked again a
  // minute later, and again after that, rather than once and never.
  it("keeps asking at a floor when the lapse is already past here", async () => {
    try {
      const fetchMock = await refetchesAfter((now) =>
        new Date(now - 5_000).toISOString(),
      );
      await vi.advanceTimersByTimeAsync(REFRESH_FLOOR_MS + 1_000);
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
      await vi.advanceTimersByTimeAsync(REFRESH_FLOOR_MS + 1_000);
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    } finally {
      vi.useRealTimers();
    }
  });

  // A lapse a month away is past what a timer can hold; it is asked about at
  // the ceiling instead of at once.
  it("waits no longer than the ceiling for a distant lapse", async () => {
    try {
      const fetchMock = await refetchesAfter((now) =>
        new Date(now + 30 * 24 * 3_600_000).toISOString(),
      );
      await vi.advanceTimersByTimeAsync(REFRESH_CEILING_MS - 60_000);
      expect(fetchMock).toHaveBeenCalledTimes(1);
      await vi.advanceTimersByTimeAsync(61_000);
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    } finally {
      vi.useRealTimers();
    }
  });
});
