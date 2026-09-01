/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
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
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { AccessPreviewPanel, TeamsCard } from "./users-access";

// The access preview and the teams roster are one file because they answer the
// same question from two sides: what a seat will reach, and the membership that
// decides it. Both are asserted against what the SERVER said — the screen never
// re-derives a role.

type Preview = {
  row_scope: "own" | "team" | "all";
  teams?: { id: string; name: string }[];
  objects?: Record<
    string,
    { read?: boolean; create?: boolean; update?: boolean; delete?: boolean }
  >;
  field_masks?: { object: string; field: string; condition: string }[];
};

type Team = { id: string; name: string; member_count?: number };
// The roster row the membership editor reads. `team_ids` is the admin-only
// field the server populates, and a fixture that omits it models a NON-admin
// read — which is a different case, not a lighter one.
type RosterUser = {
  id: string;
  email: string;
  display_name: string;
  status: string;
  is_agent: boolean;
  team_ids?: string[];
};

// Held in a constant rather than written inline: the prop is the seat's ROLE,
// and a literal here reads to the a11y lint as an ARIA role on an element.
const REP: Parameters<typeof AccessPreviewPanel>[0]["role"] = "rep";

type Call = { method: string; path: string; body: unknown };

function backend(
  opts: Readonly<{
    preview?: Preview;
    teams?: Team[];
    /** The user roster, which is where a team's membership is read from. */
    users?: RosterUser[];
    /** Answers one write with a problem document, to drive the refusal arms. */
    refuse?: (call: Call) => boolean;
    /**
     * The caller's own roles, off `/me` — admin by default, so the write
     * assertions in this suite exercise the same seat they always have. A
     * test naming `["ops"]` or `[]` here gets the read-only case, which is a
     * different render entirely rather than the same one with fewer writes
     * attempted.
     */
    me?: readonly string[];
  }>,
) {
  const calls: Call[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      // Built as a Request rather than read off `init`: openapi-fetch may pass a
      // Request with no init at all, and a mock reading the method from init
      // would answer every write as if it were a read.
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const path = new URL(request.url, "http://localhost").pathname;
      calls.push({
        method: request.method,
        path,
        // A membership write carries NO body — the ids are the path. Reading
        // one unconditionally throws, and the mock would then answer the
        // write as a network failure that looks exactly like a refusal.
        body:
          request.method === "GET" ||
          request.headers.get("content-type") === null
            ? undefined
            : await request.clone().json(),
      });
      if (path.endsWith("/me")) {
        // `user` is required on MeResponse — useMe() treats a payload
        // without it as an availability failure and never resolves `.data`,
        // which would silently read every seat here as non-admin.
        return new Response(
          JSON.stringify({
            user: { email: "you@acme.test" },
            roles: opts.me ?? ["admin"],
          }),
          { headers: { "Content-Type": "application/json" } },
        );
      }
      if (opts.refuse?.(calls[calls.length - 1])) {
        return new Response(JSON.stringify({ detail: "the team was merged" }), {
          status: 409,
          headers: { "Content-Type": "application/problem+json" },
        });
      }
      // The teams list answers as the contract answers: a page, with the
      // cursor of the next one. Without `page` the roster walk has nothing to
      // read the end of the list from.
      // Three reads answer here, and the roster two of them serve are
      // DIFFERENT lists: a test that answered users with the team page would
      // let a membership assertion pass against rows that carry no membership.
      const body = path.endsWith("/users/access-preview")
        ? (opts.preview ?? { row_scope: "own" })
        : {
            data: path.endsWith("/users")
              ? (opts.users ?? [])
              : (opts.teams ?? []),
            page: { next_cursor: null, has_more: false },
          };
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    },
  );
  return { fetchMock, calls };
}

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The region is the shell's in the running app (`main.tsx`), so a
            suite whose subject includes what an archive SAYS mounts it the
            same way — the Undo this card offers lives inside it. */}
        <ToastProvider>
          {children}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AccessPreviewPanel", () => {
  it("states the row scope, the verbs per object and every field mask the server named", async () => {
    const { fetchMock, calls } = backend({
      preview: {
        row_scope: "team",
        teams: [{ id: "t-1", name: "Nord" }],
        objects: {
          person: { read: true },
          deal: { read: true, update: true, delete: true },
        },
        field_masks: [
          { object: "deal", field: "amount", condition: "outside_scope" },
        ],
      },
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <AccessPreviewPanel role={REP} teamIds={["t-1"]} />
      </Providers>,
    );

    await screen.findByText(en["users.access.identity"]);
    // The team the scope names, not just the word "team": a preview that lost
    // the roster would still read as a team scope.
    expect(
      screen.getByText(
        en["users.access.writesTeam"].replace("{teams}", "Nord"),
      ),
    ).toBeTruthy();
    // Read alone, and read·write·delete — the verbs are derived from the grant
    // the server returned rather than from the role's name.
    expect(
      screen.getByText(`${en["users.access.object.person"]}: read`),
    ).toBeTruthy();
    expect(
      screen.getByText(
        `${en["users.access.object.deal"]}: read · write · delete`,
      ),
    ).toBeTruthy();
    // An object the server said nothing about is "no access", not silence.
    expect(
      screen.getByText(
        `${en["users.access.object.project"]}: ${en["users.access.none"]}`,
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        en["users.access.mask"]
          .replace("{field}", "deal.amount")
          .replace("{when}", en["users.access.maskOutside"]),
      ),
    ).toBeTruthy();
    // The role and the teams are what the server evaluates, so they have to
    // reach it.
    expect(calls[0]?.body).toEqual({ role: "rep", team_ids: ["t-1"] });
  });

  // A team scope with no team is a real posture — the seat is on no team yet —
  // and it must not read as if it edited every record.
  it("says a team scope with no team edits only their own records", async () => {
    const { fetchMock } = backend({
      preview: { row_scope: "team", teams: [] },
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <AccessPreviewPanel role={REP} teamIds={[]} />
      </Providers>,
    );

    expect(
      await screen.findByText(en["users.access.writesTeamNone"]),
    ).toBeTruthy();
  });
});

describe("TeamsCard", () => {
  it("counts a team of one in the singular", async () => {
    const { fetchMock } = backend({
      teams: [
        { id: "t-1", name: "Nord", member_count: 1 },
        { id: "t-2", name: "Süd", member_count: 4 },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );

    await screen.findByText("Nord");
    expect(screen.getByText("1 member")).toBeTruthy();
    expect(screen.getByText("4 members")).toBeTruthy();
  });

  it("reads as empty rather than as a failed read when no team exists", async () => {
    const { fetchMock } = backend({ teams: [] });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );

    expect(await screen.findByText(en["users.noTeamsYet"])).toBeTruthy();
  });

  it("archives the team the verb names", async () => {
    const user = userEvent.setup();
    const { fetchMock, calls } = backend({
      teams: [{ id: "t-1", name: "Nord", member_count: 2 }],
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );

    await screen.findByText("Nord");
    await user.click(
      screen.getByRole("button", {
        name: en["users.archiveTeam"].replace("{name}", "Nord"),
      }),
    );

    await waitFor(() =>
      expect(calls.some((call) => call.method === "PATCH")).toBe(true),
    );
    const patch = calls.find((call) => call.method === "PATCH");
    expect(patch?.path).toContain("/teams/t-1");
    expect(patch?.body).toEqual({ archived: true });
  });

  // A team is the one archive in this product with a way back, so it is the one
  // place the word Undo means what it says. Both arms are pinned: the restore
  // that lands, and the one the server refuses.
  it("puts an archived team back through the Undo the confirmation carries", async () => {
    const user = userEvent.setup();
    const { fetchMock, calls } = backend({
      teams: [{ id: "t-1", name: "Nord", member_count: 2 }],
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );

    await screen.findByText("Nord");
    await user.click(
      screen.getByRole("button", {
        name: en["users.archiveTeam"].replace("{name}", "Nord"),
      }),
    );

    const said = await screen.findByRole("status");
    // The NAME, not a uuid: the archive invalidated ["teams"], so by the time
    // this is read the row it came from may already be gone from the roster.
    expect(said).toHaveTextContent(
      en["users.teamArchived"].replace("{name}", "Nord"),
    );
    await user.click(
      within(said).getByRole("button", { name: en["common.undo"] }),
    );

    await waitFor(() =>
      expect(calls.filter((call) => call.method === "PATCH")).toHaveLength(2),
    );
    expect(calls.filter((call) => call.method === "PATCH")[1].body).toEqual({
      archived: false,
    });
    expect(await screen.findByRole("status")).toHaveTextContent(
      en["users.teamRestored"].replace("{name}", "Nord"),
    );
  });

  it("says so when the restore is refused, rather than letting it fail quietly", async () => {
    // The message the Undo was offered from is consumed by the press, so a
    // silent refusal leaves the reader watching a confirmation disappear and
    // believing the team came back.
    const user = userEvent.setup();
    const { fetchMock } = backend({
      teams: [{ id: "t-1", name: "Nord", member_count: 2 }],
      refuse: (call) =>
        call.method === "PATCH" &&
        (call.body as { archived: boolean }).archived === false,
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );

    await screen.findByText("Nord");
    await user.click(
      screen.getByRole("button", {
        name: en["users.archiveTeam"].replace("{name}", "Nord"),
      }),
    );
    await user.click(
      within(await screen.findByRole("status")).getByRole("button", {
        name: en["common.undo"],
      }),
    );

    expect(await screen.findByText("the team was merged")).toBeTruthy();
  });

  it("creates a team through the dialog its title verb opens, trimming the name", async () => {
    const user = userEvent.setup();
    const { fetchMock, calls } = backend({ teams: [] });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );

    await screen.findByText(en["users.noTeamsYet"]);
    await user.click(
      screen.getByRole("button", { name: en["users.newTeamOpen"] }),
    );
    const dialog = screen.getByRole("dialog");
    const submit = within(dialog).getByRole("button", {
      name: en["users.createTeam"],
    }) as HTMLButtonElement;
    // Nothing typed is nothing to create: the submit is inert until the field
    // holds a name.
    expect(submit.disabled).toBe(true);
    await user.type(
      within(dialog).getByLabelText(en["users.teamNameLabel"], {
        exact: false,
      }),
      "  Nord  ",
    );
    await user.click(submit);

    await waitFor(() =>
      expect(calls.some((call) => call.method === "POST")).toBe(true),
    );
    // Trimmed, because a team named with a trailing space is a team nobody can
    // find by typing its name.
    expect(calls.find((call) => call.method === "POST")?.body).toEqual({
      name: "Nord",
    });
    // The dialog closes on the write that landed.
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});

// Membership was fixed at invite until now: the two endpoints that change it
// existed server-side and nothing in the product reached them. These pin both
// directions, because a toggle that only ever adds looks identical to a working
// one until somebody tries to remove.
describe("TeamsCard membership", () => {
  const ROSTER: RosterUser[] = [
    {
      id: "u-in",
      email: "in@acme.test",
      display_name: "Ada Inside",
      status: "active",
      is_agent: false,
      team_ids: ["t-1"],
    },
    {
      id: "u-out",
      email: "out@acme.test",
      display_name: "Bo Outside",
      status: "active",
      is_agent: false,
      team_ids: [],
    },
  ];

  async function openTeam() {
    const user = userEvent.setup();
    await screen.findByText("Nord");
    await user.click(screen.getByText("Nord"));
    return user;
  }

  it("ticks the users already in the team and leaves the others clear", async () => {
    const { fetchMock } = backend({
      teams: [{ id: "t-1", name: "Nord", member_count: 1 }],
      users: ROSTER,
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );
    await openTeam();

    expect(
      (
        await screen.findByRole("checkbox", { name: "Ada Inside" })
      ).getAttribute("checked") !== null ||
        (
          screen.getByRole("checkbox", {
            name: "Ada Inside",
          }) as HTMLInputElement
        ).checked,
    ).toBe(true);
    expect(
      (screen.getByRole("checkbox", { name: "Bo Outside" }) as HTMLInputElement)
        .checked,
    ).toBe(false);
  });

  it("adds a user to the team the row names", async () => {
    const { fetchMock, calls } = backend({
      teams: [{ id: "t-1", name: "Nord", member_count: 1 }],
      users: ROSTER,
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );
    const user = await openTeam();

    await user.click(
      await screen.findByRole("checkbox", { name: "Bo Outside" }),
    );

    await waitFor(() =>
      expect(calls.some((call) => call.method === "PUT")).toBe(true),
    );
    const put = calls.find((call) => call.method === "PUT");
    expect(put?.path).toBe("/v1/teams/t-1/members/u-out");
  });

  it("removes a user the team already holds", async () => {
    const { fetchMock, calls } = backend({
      teams: [{ id: "t-1", name: "Nord", member_count: 1 }],
      users: ROSTER,
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );
    const user = await openTeam();

    await user.click(
      await screen.findByRole("checkbox", { name: "Ada Inside" }),
    );

    await waitFor(() =>
      expect(calls.some((call) => call.method === "DELETE")).toBe(true),
    );
    const gone = calls.find((call) => call.method === "DELETE");
    expect(gone?.path).toBe("/v1/teams/t-1/members/u-in");
  });

  it("says a refused membership write did not land", async () => {
    const { fetchMock } = backend({
      teams: [{ id: "t-1", name: "Nord", member_count: 1 }],
      users: ROSTER,
      refuse: (call) => call.method === "PUT",
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );
    const user = await openTeam();

    await user.click(
      await screen.findByRole("checkbox", { name: "Bo Outside" }),
    );

    expect(await screen.findByText("the team was merged")).toBeTruthy();
  });

  // Team membership is admin surface, so a non-admin gets no membership
  // control and no membership LIST: the roster handler only sends `team_ids`
  // to an admin caller (`WithRoles: isAdmin`), so a read-only render built
  // from an ops seat's own roster read would show nobody as a member of
  // anything — a false statement, not an honest withholding. The card states
  // why once, and the disclosure body says the same thing rather than
  // fabricate a list.
  it("withholds membership entirely from a seat that may not read or change it", async () => {
    const { fetchMock, calls } = backend({
      teams: [{ id: "t-1", name: "Nord", member_count: 1 }],
      users: ROSTER,
      me: ["ops"],
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );
    await openTeam();

    expect(
      await screen.findByText(
        `${en["users.teamsSub"]} ${en["users.teamsAdminOnly"]}`,
      ),
    ).toBeTruthy();
    expect(screen.getByText(en["users.teamMembersAdminOnly"])).toBeTruthy();
    expect(screen.queryByText("Ada Inside")).toBeNull();
    expect(screen.queryByText("Bo Outside")).toBeNull();
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(screen.queryByRole("button", { name: /archive/i })).toBeNull();

    // No membership read fires either: there is nothing in it this seat
    // would be shown, so there is nothing to walk the roster for.
    expect(calls.some((call) => call.path.endsWith("/users"))).toBe(false);
    expect(calls.some((call) => call.method === "PUT")).toBe(false);
    expect(calls.some((call) => call.method === "DELETE")).toBe(false);
    expect(calls.some((call) => call.method === "PATCH")).toBe(false);
  });
});
