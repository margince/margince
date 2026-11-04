/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useEffect, useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { jsonResponse, render } from "../../testing/settings.testkit";
import {
  NewTeamAction,
  RenameTeamAction,
  TeamMembersModal,
  type TeamUser,
} from "./team-dialogs";

// A team name is unique, and the 409 detail names the store, not the reader's
// next step.
const TAKEN = {
  type: "about:blank",
  title: "Conflict",
  status: 409,
  code: "conflict",
  detail: 'conflict: a team named "Nord" already exists',
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it.each<[string, ReactNode, string]>([
  ["create", <NewTeamAction key="create" />, en["users.newTeamOpen"]],
  [
    "rename",
    <RenameTeamAction key="rename" team={{ id: "t-1", name: "Süd" }} />,
    en["users.teamRename"],
  ],
])(
  "a %s refused as a duplicate says so on the name field until it changes",
  async (_verb, action, opener) => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(TAKEN, 409)),
    );
    render(action);
    await user.click(screen.getByRole("button", { name: opener }));
    const dialog = within(await screen.findByRole("dialog"));
    const field = dialog.getByLabelText(en["users.teamNameLabel"]);
    await user.clear(field);
    await user.type(field, "Nord{Enter}");

    await vi.waitFor(() =>
      expect(field).toHaveAccessibleDescription(en["users.teamDuplicate"]),
    );
    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(dialog.queryByText(TAKEN.detail)).toBeNull();
    await user.type(field, "s");
    expect(dialog.queryByText(en["users.teamDuplicate"])).toBeNull();
  },
);

// A team of 120 is searched by name. The search narrows the rows, a removal
// takes off the member on the pressed row, and focus lands on the next row the
// reader can still see.
const TEAM = { id: "t-dach", name: "DACH Sales" } as const;

function teamOf(size: number): TeamUser[] {
  return Array.from({ length: size }, (_, at) => {
    const n = String(at).padStart(3, "0");
    return {
      id: `u-${n}`,
      display_name: `Roster Member ${n}`,
      email: `roster-${n}@example.test`,
      status: "active",
      team_ids: [TEAM.id],
    } as TeamUser;
  });
}

// Holds the roster as the server would after a removal lands.
let dropMember: (id: string) => void = () => {};
function Roster({ size }: Readonly<{ size: number }>) {
  const [users, setUsers] = useState(() => teamOf(size));
  useEffect(() => {
    dropMember = (id) =>
      setUsers((all) =>
        all.map((u) => (u.id === id ? { ...u, team_ids: [] } : u)),
      );
  }, []);
  return (
    <TeamMembersModal
      team={{ ...TEAM, member_count: users.length } as never}
      users={users}
      usersPartial={false}
      canEdit
      open
      onClose={() => {}}
    />
  );
}

it("searches a team of 120 and removes the member on the row pressed", async () => {
  const removed: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.method === "DELETE") {
        const id = new URL(req.url).pathname.split("/").at(-1) ?? "";
        removed.push(id);
        dropMember(id);
        return new Response(null, { status: 204 });
      }
      return jsonResponse({ data: [] });
    }),
  );
  const user = userEvent.setup();
  render(<Roster size={120} />);
  const dialog = within(await screen.findByRole("dialog"));
  const rows = () => dialog.queryAllByText(/^Roster Member \d{3}$/);
  expect(rows()).toHaveLength(120);

  await user.type(
    dialog.getByRole("searchbox", { name: en["teams.searchMembers"] }),
    "Roster Member 07",
  );
  // "07" is a word of 007, 070 to 079 and 107: twelve rows, in name order.
  const expected = teamOf(120)
    .map((u) => u.display_name)
    .filter((name) => name.includes("07"));
  expect(expected).toHaveLength(12);
  expect(rows().map((row) => row.textContent)).toEqual(expected);
  expect(dialog.getByText("12 of 120 members")).toBeInTheDocument();

  await user.click(
    dialog.getByRole("button", {
      name: "Remove Roster Member 072 from DACH Sales",
    }),
  );
  await vi.waitFor(() => expect(removed).toEqual(["u-072"]));
  await vi.waitFor(() =>
    expect(document.activeElement).toBe(
      dialog.getByRole("button", {
        name: "Remove Roster Member 073 from DACH Sales",
      }),
    ),
  );
});

// A search typed while a removal is in flight keeps focus in the search box.
// Focus moves only once the removal has landed.
//
// Mutation: move focus on any change in the shown rows, as before, and the
// search box loses focus while the removal waits.
it("leaves focus in the search box while a removal is pending", async () => {
  let answer: (() => void) | undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.method === "DELETE") {
        const id = new URL(req.url).pathname.split("/").at(-1) ?? "";
        await new Promise<void>((resolve) => {
          answer = () => {
            dropMember(id);
            resolve();
          };
        });
        return new Response(null, { status: 204 });
      }
      return jsonResponse({ data: [] });
    }),
  );
  const user = userEvent.setup();
  render(<Roster size={120} />);
  const dialog = within(await screen.findByRole("dialog"));
  const search = dialog.getByRole("searchbox", {
    name: en["teams.searchMembers"],
  });
  await user.type(search, "Roster Member 07");
  await user.click(
    dialog.getByRole("button", {
      name: "Remove Roster Member 072 from DACH Sales",
    }),
  );
  await user.click(search);
  await user.type(search, "{Backspace}");
  expect(search).toHaveFocus();
  await vi.waitFor(() => expect(answer).toBeDefined());
  answer?.();
  await vi.waitFor(() => expect(search).not.toHaveFocus());
});

// The count's total is the team's own, not however much of the roster this
// reader's list loaded. A roster cut short says so.
it("counts against the team's size when the roster is cut short", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse({ data: [] })),
  );
  const user = userEvent.setup();
  render(
    <TeamMembersModal
      team={{ ...TEAM, member_count: 120 } as never}
      users={teamOf(50)}
      usersPartial
      canEdit
      open
      onClose={() => {}}
    />,
  );
  const dialog = within(await screen.findByRole("dialog"));
  await user.type(
    dialog.getByRole("searchbox", { name: en["teams.searchMembers"] }),
    "Roster Member 01",
  );
  expect(dialog.getByText(/ of 120 members$/)).toBeInTheDocument();
  expect(dialog.queryByText(/ of 50 members$/)).toBeNull();
});
