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
import { AccessPreviewPanel, TeamsCard } from "./users-access";
import { backend, Providers } from "./users-access.testkit";

// The access preview and the teams roster are one file because they answer the
// same question from two sides: what a seat will reach, and the membership that
// decides it. Both are asserted against what the SERVER said — the screen never
// re-derives a role.

const REP: Parameters<typeof AccessPreviewPanel>[0]["role"] = "rep";

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

    // Only the kinds the server granted read on, never a fixed list.
    await screen.findByText(
      en["users.access.reads"].replace("{kinds}", "contacts and deals"),
    );
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
      preview: {
        row_scope: "team",
        teams: [],
        objects: { contact: { read: true, update: true } },
      },
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
      name: en["table.rowActions"].replace("{name}", "Nord"),
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
        name: en["table.rowActions"].replace("{name}", "Nord"),
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

  // A refetch that fails keeps the teams it had, so the verb stays with them.
  it("keeps New team while a re-read of the teams fails", async () => {
    const user = userEvent.setup();
    let teamReads = 0;
    const { fetchMock } = backend({
      teams: [{ id: "t-1", name: "Nord", member_count: 2 }],
      refuse: (call) =>
        call.method === "GET" &&
        call.path.endsWith("/teams") &&
        teamReads++ > 0,
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <TeamsCard />
      </Providers>,
    );

    await archiveVia(user);

    await waitFor(() => expect(teamReads).toBeGreaterThan(1));
    await screen.findByRole("button", { name: /try again|retry/i });
    expect(
      screen.getByRole("button", { name: en["users.newTeamOpen"] }),
    ).toBeTruthy();
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
    expect(await within(first).findByText("the team was merged")).toBeTruthy();
    await user.keyboard("{Escape}");
    await user.click(
      screen.getByRole("button", { name: en["unsaved.discard"] }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    const second = await open();
    expect(within(second).queryByText("the team was merged")).toBeNull();
  });
});
