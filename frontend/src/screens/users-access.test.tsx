/** @vitest-environment happy-dom */
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

type Team = {
  id: string;
  name: string;
  member_count?: number;
  parent_team_id?: string;
};
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

type Call = {
  method: string;
  path: string;
  body: unknown;
  // The query, because a GET carries what a POST used to put in its body.
  query: Record<string, string>;
};

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
    /**
     * The grants this principal holds, when a case needs something other than
     * the full pair. The card asks two DIFFERENT objects — `team_admin` for the
     * verbs and `user_admin:read` for the membership the roster carries — so a
     * case can now describe a reader who sees who is in a team and may not
     * change it, which one admin check could not express.
     */
    allow?: Record<string, Record<string, boolean>>;
    /** Held open until it settles, so a case can look at a write in flight. */
    holdWrites?: Promise<void>;
  }>,
) {
  const calls: Call[] = [];
  // Copied, so a membership write lands on this case's roster and the refetch
  // after it reads the team the way the server would.
  const users = (opts.users ?? []).map((user) => ({
    ...user,
    team_ids: user.team_ids && [...user.team_ids],
  }));
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      // Built as a Request rather than read off `init`: openapi-fetch may pass a
      // Request with no init at all, and a mock reading the method from init
      // would answer every write as if it were a read.
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(request.url, "http://localhost");
      const path = url.pathname;
      calls.push({
        method: request.method,
        path,
        query: Object.fromEntries(url.searchParams),
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
        //
        // The GRANTS matter for the same reason, one level up: the card asks
        // `team_admin` for its verbs and `user_admin:read` for the membership
        // the roster carries, so a payload naming a role and no authorization
        // describes a principal the API cannot produce — and every case here
        // would pass with nothing rendered. `allow` lets a case withhold one
        // of the two, which is the reader the split made possible.
        return new Response(
          JSON.stringify({
            user: { email: "you@acme.test" },
            roles: opts.me ?? ["admin"],
            authorization: {
              objects: opts.allow ?? {
                team_admin: {
                  read: true,
                  create: true,
                  update: true,
                  delete: false,
                },
                user_admin: {
                  read: true,
                  create: true,
                  update: true,
                  delete: true,
                },
              },
              seat_type: "full",
              row_scope: "all",
            },
          }),
          { headers: { "Content-Type": "application/json" } },
        );
      }
      if (request.method !== "GET") {
        await opts.holdWrites;
      }
      if (opts.refuse?.(calls[calls.length - 1])) {
        return new Response(JSON.stringify({ detail: "the team was merged" }), {
          status: 409,
          headers: { "Content-Type": "application/problem+json" },
        });
      }
      applyMembership(users, request.method, path);
      // The teams list answers as the contract answers: a page, with the
      // cursor of the next one. Without `page` the roster walk has nothing to
      // read the end of the list from.
      // Three reads answer here, and the roster two of them serve are
      // DIFFERENT lists: a test that answered users with the team page would
      // let a membership assertion pass against rows that carry no membership.
      const body = path.endsWith("/users/access-preview")
        ? (opts.preview ?? { row_scope: "own" })
        : {
            data: path.endsWith("/users") ? users : (opts.teams ?? []),
            page: { next_cursor: null, has_more: false },
          };
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    },
  );
  return { fetchMock, calls };
}

function applyMembership(users: RosterUser[], method: string, path: string) {
  const [, teamId, userId] =
    /\/teams\/([^/]+)\/members\/([^/]+)$/.exec(path) ?? [];
  const user = users.find((each) => each.id === userId);
  if (!(teamId && user && (method === "PUT" || method === "DELETE"))) {
    return;
  }
  const others = (user.team_ids ?? []).filter((id) => id !== teamId);
  user.team_ids = method === "PUT" ? [...others, teamId] : others;
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
          contact: { read: true },
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
      screen.getByText(`${en["users.access.object.contact"]}: read`),
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
    // reach it — in the QUERY, because the preview is a GET. That method is
    // load-bearing rather than incidental: the seat ceiling is method-based,
    // so a read wearing POST is refused to a read-seat admin, whose whole
    // purpose is reading.
    expect(calls[0]?.method).toBe("GET");
    expect(calls[0]?.query).toEqual({ role: "rep", team_ids: "t-1" });
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

// Archiving sits in the row's menu; the menu is named for the team it acts on.
async function archiveVia(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", {
      name: en["users.teamRowActions"].replace("{name}", "Nord"),
    }),
  );
  await user.click(
    await screen.findByRole("button", { name: en["users.teamArchive"] }),
  );
}

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

  it("names a team's parent under its name", async () => {
    const { fetchMock } = backend({
      teams: [
        { id: "t-1", name: "Nord", member_count: 1 },
        { id: "t-2", name: "Hamburg", member_count: 0, parent_team_id: "t-1" },
      ],
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );

    expect(
      await screen.findByText(en["users.teamParent"].replace("{name}", "Nord")),
    ).toBeTruthy();
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

  it("archives the team the menu names", async () => {
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

    await archiveVia(user);

    await waitFor(() =>
      expect(calls.some((call) => call.method === "PATCH")).toBe(true),
    );
    const patch = calls.find((call) => call.method === "PATCH");
    expect(patch?.path).toContain("/teams/t-1");
    expect(patch?.body).toEqual({ archived: true });
  });

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

    await archiveVia(user);

    const said = await screen.findByRole("status");
    // The name, not a uuid: the refetch may have taken the row away already.
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

    await archiveVia(user);
    await user.click(
      within(await screen.findByRole("status")).getByRole("button", {
        name: en["common.undo"],
      }),
    );

    expect(await screen.findByText("the team was merged")).toBeTruthy();
  });

  it("renames a team through the dialog its menu opens, starting from the current name", async () => {
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

    await user.click(
      await screen.findByRole("button", {
        name: en["users.teamRowActions"].replace("{name}", "Nord"),
      }),
    );
    await user.click(
      await screen.findByRole("button", { name: en["users.teamRename"] }),
    );
    const dialog = await screen.findByRole("dialog");
    const field = within(dialog).getByLabelText(en["users.teamNameLabel"], {
      exact: false,
    });
    expect(field).toHaveValue("Nord");
    const save = within(dialog).getByRole("button", {
      name: en["users.teamRenameSave"],
    });
    // The unchanged name is no rename.
    expect(save).toBeDisabled();
    await user.clear(field);
    await user.type(field, " Nordost ");
    await user.click(save);

    await waitFor(() =>
      expect(calls.some((call) => call.method === "PATCH")).toBe(true),
    );
    const patch = calls.find((call) => call.method === "PATCH");
    expect(patch?.path).toBe("/v1/teams/t-1");
    expect(patch?.body).toEqual({ name: "Nordost" });
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
    expect(calls.find((call) => call.method === "POST")?.body).toEqual({
      name: "Nord",
    });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("reopens the new-team dialog without the last attempt's refusal", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backend({
      teams: [],
      refuse: (call) => call.method === "POST",
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );
    const open = async () => {
      await user.click(
        await screen.findByRole("button", { name: en["users.newTeamOpen"] }),
      );
      return screen.getByRole("dialog");
    };

    const first = await open();
    await user.type(
      within(first).getByLabelText(en["users.teamNameLabel"], { exact: false }),
      "Nord",
    );
    await user.click(
      within(first).getByRole("button", { name: en["users.createTeam"] }),
    );
    expect(await within(first).findByText(en["users.notCreated"])).toBeTruthy();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    const second = await open();
    expect(within(second).queryByText(en["users.notCreated"])).toBeNull();
  });
});

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
    // Seats the server refuses on the way in are never offered.
    {
      id: "u-agent",
      email: "agent@acme.test",
      display_name: "Cy Agent",
      status: "active",
      is_agent: true,
      team_ids: [],
    },
  ];
  const NORD = [{ id: "t-1", name: "Nord", member_count: 1 }];

  async function openTeam() {
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Nord" }));
    return { user, dialog: await screen.findByRole("dialog") };
  }

  const removeVerb = (dialog: HTMLElement, name: string) =>
    within(dialog).getByRole("button", {
      name: en["users.teamRemoveMember"]
        .replace("{name}", name)
        .replace("{team}", "Nord"),
    });

  function mount(opts: Parameters<typeof backend>[0]) {
    const { fetchMock, calls } = backend(opts);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );
    return calls;
  }

  it("lists the team's members in a dialog titled with its name", async () => {
    mount({ teams: NORD, users: ROSTER });
    const { dialog } = await openTeam();

    expect(within(dialog).getByRole("heading", { name: "Nord" })).toBeTruthy();
    const list = within(dialog).getByRole("list", {
      name: en["users.teamMembersLabel"],
    });
    expect(within(list).getByText("Ada Inside")).toBeTruthy();
    expect(within(list).queryByText("Bo Outside")).toBeNull();
  });

  it("offers only the active colleagues not yet on the team", async () => {
    mount({ teams: NORD, users: ROSTER });
    const { user, dialog } = await openTeam();

    await user.click(
      within(dialog).getByRole("combobox", { name: en["users.teamAddMember"] }),
    );
    const offered = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toHaveLength(1);
    expect(offered[0]).toContain("Bo Outside");
  });

  it("adds the colleague picked to the team the dialog names", async () => {
    const calls = mount({ teams: NORD, users: ROSTER });
    const { user, dialog } = await openTeam();

    await user.type(
      within(dialog).getByRole("combobox", { name: en["users.teamAddMember"] }),
      "Bo",
    );
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: /Bo Outside/,
      }),
    );

    await waitFor(() =>
      expect(calls.some((call) => call.method === "PUT")).toBe(true),
    );
    expect(calls.find((call) => call.method === "PUT")?.path).toBe(
      "/v1/teams/t-1/members/u-out",
    );
  });

  it("removes the member whose verb is pressed", async () => {
    const calls = mount({ teams: NORD, users: ROSTER });
    const { user, dialog } = await openTeam();

    await user.click(
      within(dialog).getByRole("button", {
        name: en["users.teamRemoveMember"]
          .replace("{name}", "Ada Inside")
          .replace("{team}", "Nord"),
      }),
    );

    await waitFor(() =>
      expect(calls.some((call) => call.method === "DELETE")).toBe(true),
    );
    expect(calls.find((call) => call.method === "DELETE")?.path).toBe(
      "/v1/teams/t-1/members/u-in",
    );
  });

  it("says a refused membership write did not land and keeps the dialog as it was", async () => {
    mount({
      teams: NORD,
      users: ROSTER,
      refuse: (call) => call.method === "DELETE",
    });
    const { user, dialog } = await openTeam();

    await user.click(
      within(dialog).getByRole("button", {
        name: en["users.teamRemoveMember"]
          .replace("{name}", "Ada Inside")
          .replace("{team}", "Nord"),
      }),
    );

    expect(await within(dialog).findByText("the team was merged")).toBeTruthy();
    expect(within(dialog).getByText(en["users.teamNotChanged"])).toBeTruthy();
    expect(screen.getByRole("dialog")).toBe(dialog);
    expect(within(dialog).getByText("Ada Inside")).toBeTruthy();
    expect(document.activeElement).toBe(removeVerb(dialog, "Ada Inside"));
  });

  describe("focus across a membership write", () => {
    const TEAM_OF_TWO: RosterUser[] = [
      ...ROSTER,
      {
        id: "u-in-2",
        email: "ben@acme.test",
        display_name: "Ben Inside",
        status: "active",
        is_agent: false,
        team_ids: ["t-1"],
      },
      {
        id: "u-out-2",
        email: "di@acme.test",
        display_name: "Di Outside",
        status: "active",
        is_agent: false,
        team_ids: [],
      },
    ];

    function heldWrites() {
      let release = () => {};
      const holdWrites = new Promise<void>((resolve) => {
        release = resolve;
      });
      return { holdWrites, release };
    }

    async function pickColleague(
      user: ReturnType<typeof userEvent.setup>,
      field: HTMLElement,
      name: string,
    ) {
      await user.clear(field);
      await user.type(field, name.slice(0, 2));
      await user.click(
        within(screen.getByRole("listbox")).getByRole("option", {
          name: new RegExp(name),
        }),
      );
    }

    it("keeps the add field enabled and focused, and takes one pick at a time", async () => {
      const { holdWrites, release } = heldWrites();
      const calls = mount({ teams: NORD, users: TEAM_OF_TWO, holdWrites });
      const { user, dialog } = await openTeam();
      const field = within(dialog).getByRole("combobox", {
        name: en["users.teamAddMember"],
      });

      await pickColleague(user, field, "Bo Outside");
      await waitFor(() =>
        expect(calls.filter((call) => call.method === "PUT")).toHaveLength(1),
      );
      expect(field).toBeEnabled();
      expect(document.activeElement).toBe(field);
      await pickColleague(user, field, "Di Outside");
      release();

      const list = within(dialog).getByRole("list", {
        name: en["users.teamMembersLabel"],
      });
      expect(await within(list).findByText("Bo Outside")).toBeTruthy();
      expect(calls.filter((call) => call.method === "PUT")).toHaveLength(1);
      expect(document.activeElement).toBe(field);
    });

    it("keeps the pressed remove verb focused while it writes, then lands on the row in its place", async () => {
      const { holdWrites, release } = heldWrites();
      mount({ teams: NORD, users: TEAM_OF_TWO, holdWrites });
      const { user, dialog } = await openTeam();
      const pressed = removeVerb(dialog, "Ada Inside");

      await user.click(pressed);
      await waitFor(() => expect(pressed).toHaveAttribute("aria-busy", "true"));
      expect(pressed).toBeEnabled();
      expect(document.activeElement).toBe(pressed);
      expect(removeVerb(dialog, "Ben Inside")).toBeDisabled();
      release();

      await waitFor(() =>
        expect(within(dialog).queryByText("Ada Inside")).toBeNull(),
      );
      expect(document.activeElement).toBe(removeVerb(dialog, "Ben Inside"));
    });
  });

  it("closes on Escape and gives focus back to the name that opened it", async () => {
    mount({ teams: NORD, users: ROSTER });
    const { user } = await openTeam();

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Nord" }),
    );
  });

  it("shows membership without its verbs to a seat holding the roster read and no team verb", async () => {
    mount({
      teams: NORD,
      users: ROSTER,
      me: ["custom"],
      allow: {
        user_admin: { read: true, create: false, update: false, delete: false },
      },
    });
    const { dialog } = await openTeam();

    expect(within(dialog).getByText("Ada Inside")).toBeTruthy();
    expect(
      within(dialog).getByText(en["users.teamMembersReadOnly"]),
    ).toBeTruthy();
    expect(within(dialog).queryByRole("combobox")).toBeNull();
    expect(
      within(dialog).queryByRole("button", { name: /Ada Inside/ }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: en["users.teamRowActions"].replace("{name}", "Nord"),
      }),
    ).toBeNull();
  });

  // Only an admin changes who is on a team or archives it (identity/teams.go);
  // renaming needs the team write alone.
  it("offers a team_admin holder who is not an admin the rename and nothing else", async () => {
    mount({
      teams: NORD,
      users: ROSTER,
      me: ["custom"],
      allow: {
        team_admin: { read: true, create: true, update: true, delete: false },
        user_admin: { read: true, create: false, update: false, delete: false },
      },
    });
    const { user, dialog } = await openTeam();

    expect(within(dialog).queryByRole("combobox")).toBeNull();
    await user.keyboard("{Escape}");
    await user.click(
      await screen.findByRole("button", {
        name: en["users.teamRowActions"].replace("{name}", "Nord"),
      }),
    );
    expect(
      screen.getByRole("button", { name: en["users.teamRename"] }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["users.teamArchive"] }),
    ).toBeNull();
  });

  it("withholds membership entirely from a seat that may not read or change it", async () => {
    const calls = mount({
      teams: NORD,
      users: ROSTER,
      me: ["ops"],
      allow: {},
    });

    expect(
      await screen.findByText(
        `${en["users.teamsSub"]} ${en["users.teamsAdminOnly"]} ${en["users.teamMembersAdminOnly"]}`,
      ),
    ).toBeTruthy();
    expect(screen.getByText("Nord")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Nord" })).toBeNull();
    expect(screen.queryByText("Ada Inside")).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: en["users.teamRowActions"].replace("{name}", "Nord"),
      }),
    ).toBeNull();
    // Nothing in the user roster would be shown, so it is never walked.
    expect(calls.some((call) => call.path.endsWith("/users"))).toBe(false);
  });
});
