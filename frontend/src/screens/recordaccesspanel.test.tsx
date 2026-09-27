/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { WhoCanSeePanel } from "./recordaccesspanel";

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

function serve(body: RecordAccess) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = input instanceof Request ? input.url : String(input);
    if (url.includes("/contacts/c-1/access")) {
      return new Response(JSON.stringify(body), {
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

  it("says how many colleagues the page did not list", async () => {
    serve(answer({ page: { has_more: true, total: 205 } }));
    render(<WhoCanSeePanel kind="contact" recordId="c-1" />);

    expect(
      await screen.findByText("200 more users are not listed."),
    ).toBeTruthy();
  });

  it("reads the answer again when the earliest share lapses", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const lapse = new Date(Date.now() + 60_000).toISOString();
      const fetchMock = serve(answer({ refresh_at: lapse }));
      render(<WhoCanSeePanel kind="contact" recordId="c-1" />);
      await screen.findByTestId("who-can-see-you");
      expect(fetchMock).toHaveBeenCalledTimes(1);

      await vi.advanceTimersByTimeAsync(61_000);
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    } finally {
      vi.useRealTimers();
    }
  });
});
