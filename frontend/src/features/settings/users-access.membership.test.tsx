/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { TeamsCard } from "./users-access";
import { backend, Providers, type RosterUser } from "./users-access.testkit";

// Who a team lists as a member, and the writes that change it.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
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

  it("lists an invited member with an Invited mark and lets the admin remove them", async () => {
    const calls = mount({
      teams: [{ id: "t-1", name: "Nord", member_count: 2 }],
      users: [
        ...ROSTER,
        {
          id: "u-inv",
          email: "new@acme.test",
          display_name: "Di Invited",
          status: "invited",
          is_agent: false,
          team_ids: ["t-1"],
        },
        {
          id: "u-gone",
          email: "gone@acme.test",
          display_name: "Ed Gone",
          status: "deactivated",
          is_agent: false,
          team_ids: ["t-1"],
        },
      ],
    });
    const { user, dialog } = await openTeam();

    const list = within(dialog).getByRole("list", {
      name: en["users.teamMembersLabel"],
    });
    expect(within(list).getByText(/new@acme.test · Invited/)).toBeTruthy();
    expect(within(list).queryByText("Ed Gone")).toBeNull();

    await user.click(removeVerb(dialog, "Di Invited"));
    await waitFor(() =>
      expect(calls.find((call) => call.method === "DELETE")?.path).toBe(
        "/v1/teams/t-1/members/u-inv",
      ),
    );
  });

  it("adds an invited seat that is not yet on the team, asking for the wide roster", async () => {
    const calls = mount({
      teams: NORD,
      users: [
        ...ROSTER,
        {
          id: "u-inv2",
          email: "later@acme.test",
          display_name: "Fay Later",
          status: "invited",
          is_agent: false,
          team_ids: [],
        },
      ],
    });
    const { user, dialog } = await openTeam();

    await user.type(
      within(dialog).getByRole("combobox", { name: en["users.teamAddMember"] }),
      "Fay",
    );
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: /Fay Later/,
      }),
    );

    await waitFor(() =>
      expect(calls.find((call) => call.method === "PUT")?.path).toBe(
        "/v1/teams/t-1/members/u-inv2",
      ),
    );
    expect(
      calls.some(
        (call) =>
          call.path.endsWith("/users") &&
          call.query?.include_inactive === "true",
      ),
    ).toBe(true);
  });

  it("does not list an agent seat that is on the team", async () => {
    mount({
      teams: NORD,
      users: [
        ...ROSTER,
        {
          id: "u-bot",
          email: "bot@acme.test",
          display_name: "Dot Bot",
          status: "active",
          is_agent: true,
          team_ids: ["t-1"],
        },
      ],
    });
    const { dialog } = await openTeam();

    const list = within(dialog).getByRole("list", {
      name: en["users.teamMembersLabel"],
    });
    expect(within(list).getByText("Ada Inside")).toBeTruthy();
    expect(within(list).queryByText("Dot Bot")).toBeNull();
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
        name: en["table.rowActions"].replace("{name}", "Nord"),
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
        name: en["table.rowActions"].replace("{name}", "Nord"),
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
        `${en["users.teamsSub"]} ${en["users.teamMembersAdminOnly"]}`,
      ),
    ).toBeTruthy();
    expect(screen.getByText("Nord")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Nord" })).toBeNull();
    expect(screen.queryByText("Ada Inside")).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: en["table.rowActions"].replace("{name}", "Nord"),
      }),
    ).toBeNull();
    // Nothing in the user roster would be shown, so it is never walked.
    expect(calls.some((call) => call.path.endsWith("/users"))).toBe(false);
  });
});
