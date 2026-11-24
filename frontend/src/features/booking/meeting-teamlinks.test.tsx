/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { TeamLinksCard } from "./meeting-teamlinks";

type Link = components["schemas"]["SchedulingTeamLink"];

const GRACE = "0198f011-bbbb-7000-8000-000000000001";
const ALAN = "0198f011-bbbb-7000-8000-000000000002";
const PAGE = "0198f011-bbbb-7000-8000-000000000005";

const link: Link = {
  id: PAGE,
  title: "Meet Ada Lovelace and Grace Hopper",
  enabled: true,
  slug: "team-slug",
  public_url: "https://crm.example.com/book/team-slug",
  blocker: "team_not_ready",
  team_members: [{ user_id: GRACE, name: "Grace Hopper", state: "pending" }],
};

const candidates = [
  { user_id: GRACE, name: "Grace Hopper" },
  { user_id: ALAN, name: "Alan Turing" },
];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(routes: RouteMap) {
  installFetchStub(routes);
  render(
    <StoryProviders>
      <TeamLinksCard hostName="Ada Lovelace" />
    </StoryProviders>,
  );
}

it("shows each team link with its colleagues and where they stand", async () => {
  mount({
    "GET /scheduling/team-links": () => jsonResponse([link]),
    "GET /scheduling/team-candidates": () => jsonResponse(candidates),
  });
  expect(
    await screen.findByText("Meet Ada Lovelace and Grace Hopper"),
  ).toBeTruthy();
  expect(screen.getByText("Grace Hopper")).toBeTruthy();
  expect(screen.getByText("Waiting for confirmation")).toBeTruthy();
  expect(screen.getByText("Waiting for colleagues")).toBeTruthy();
});

it("shows a switched-off link as off, not as taking bookings", async () => {
  mount({
    "GET /scheduling/team-links": () =>
      jsonResponse([{ ...link, enabled: false, blocker: "none" }]),
    "GET /scheduling/team-candidates": () => jsonResponse(candidates),
  });
  expect(await screen.findByText("Off: takes no bookings")).toBeTruthy();
  expect(screen.queryByText("Takes bookings")).toBeNull();
});

it("creates a team link named for the host and the chosen colleague", async () => {
  const user = userEvent.setup();
  const created: unknown[] = [];
  mount({
    "GET /scheduling/team-links": () => jsonResponse([]),
    "GET /scheduling/team-candidates": () => jsonResponse(candidates),
    "POST /scheduling/team-links": (body) => {
      created.push(body);
      return jsonResponse(link, 201);
    },
  });
  const picker = await screen.findByRole("combobox", {
    name: "New team link with",
  });
  await waitFor(() => expect(picker.hasAttribute("disabled")).toBe(false));
  await user.click(picker);
  await user.click(screen.getByRole("option", { name: "Alan Turing" }));
  await waitFor(() =>
    expect(created).toEqual([
      {
        title: "Meet Ada Lovelace and Alan Turing",
        team_members: [{ user_id: ALAN }],
      },
    ]),
  );
});

it("adds a colleague to a team link by id alone, and the default name follows", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount({
    "GET /scheduling/team-links": () => jsonResponse([link]),
    "GET /scheduling/team-candidates": () => jsonResponse(candidates),
    [`PUT /scheduling/team-links/${PAGE}`]: (body) => {
      writes.push(body);
      return jsonResponse(link);
    },
  });
  const picker = await screen.findByRole("combobox", {
    name: "Add a colleague",
  });
  await waitFor(() => expect(picker.hasAttribute("disabled")).toBe(false));
  await user.click(picker);
  // A colleague already on the link is not offered a second time.
  expect(screen.queryByRole("option", { name: "Grace Hopper" })).toBeNull();
  await user.click(screen.getByRole("option", { name: "Alan Turing" }));
  await waitFor(() =>
    expect(writes).toEqual([
      {
        title: "Meet Ada Lovelace, Grace Hopper and Alan Turing",
        team_members: [{ user_id: GRACE }, { user_id: ALAN }],
      },
    ]),
  );
});

it("keeps a name the host chose when the team changes", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  const named = {
    ...link,
    title: "Discovery call",
    team_members: [
      ...link.team_members,
      { user_id: ALAN, name: "Alan Turing", state: "pending" as const },
    ],
  };
  mount({
    "GET /scheduling/team-links": () => jsonResponse([named]),
    "GET /scheduling/team-candidates": () => jsonResponse(candidates),
    [`PUT /scheduling/team-links/${PAGE}`]: (body) => {
      writes.push(body);
      return jsonResponse(named);
    },
  });
  const [removeGrace] = await screen.findAllByRole("button", {
    name: "Remove",
  });
  await user.click(removeGrace);
  await waitFor(() =>
    expect(writes).toEqual([
      { title: "Discovery call", team_members: [{ user_id: ALAN }] },
    ]),
  );
});

it("deletes a team link only after the host confirms", async () => {
  const user = userEvent.setup();
  const deleted: string[] = [];
  mount({
    "GET /scheduling/team-links": () => jsonResponse([link]),
    "GET /scheduling/team-candidates": () => jsonResponse(candidates),
    [`DELETE /scheduling/team-links/${PAGE}`]: () => {
      deleted.push(PAGE);
      return new Response(null, { status: 204 });
    },
  });
  await user.click(await screen.findByRole("button", { name: "Delete link" }));
  expect(deleted).toEqual([]);
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain("Your personal link does not change.");
  const [, confirm] = screen.getAllByRole("button", { name: "Delete link" });
  await user.click(confirm);
  await waitFor(() => expect(deleted).toEqual([PAGE]));
});
