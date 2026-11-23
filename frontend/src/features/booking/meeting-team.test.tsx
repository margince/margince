/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { bookingProfile } from "../../testing/book.testkit";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { TeamMembersCard, TeamRequestsCard } from "./meeting-team";

type Profile = components["schemas"]["SchedulingProfile"];

const GRACE = "0198f011-bbbb-7000-8000-000000000001";
const ALAN = "0198f011-bbbb-7000-8000-000000000002";
const HOST = "0198f011-bbbb-7000-8000-000000000003";

const teamProfile: Profile = {
  ...bookingProfile,
  blocker: "team_not_ready",
  team_members: [{ user_id: GRACE, name: "Grace Hopper", state: "pending" }],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(node: React.ReactNode, routes: RouteMap) {
  installFetchStub(routes);
  render(<StoryProviders>{node}</StoryProviders>);
}

it("shows each colleague with where they stand", async () => {
  mount(<TeamMembersCard profile={teamProfile} />, {
    "GET /scheduling/team-candidates": () => jsonResponse([]),
  });
  expect(screen.getByText("Grace Hopper")).toBeTruthy();
  expect(screen.getByText("Waiting for confirmation")).toBeTruthy();
});

it("adds a colleague by id alone, leaving their answer to the server", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount(<TeamMembersCard profile={teamProfile} />, {
    "GET /scheduling/team-candidates": () =>
      jsonResponse([
        { user_id: GRACE, name: "Grace Hopper" },
        { user_id: ALAN, name: "Alan Turing" },
      ]),
    "GET /scheduling/profile": () => jsonResponse(teamProfile),
    "PUT /scheduling/profile": (body) => {
      writes.push((body as Profile).team_members);
      return jsonResponse(body);
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
    expect(writes).toEqual([[{ user_id: GRACE }, { user_id: ALAN }]]),
  );
});

it("removes a colleague from the link", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  mount(<TeamMembersCard profile={teamProfile} />, {
    "GET /scheduling/team-candidates": () => jsonResponse([]),
    "GET /scheduling/profile": () => jsonResponse(teamProfile),
    "PUT /scheduling/profile": (body) => {
      writes.push((body as Profile).team_members);
      return jsonResponse(body);
    },
  });
  await user.click(screen.getByRole("button", { name: "Remove" }));
  await waitFor(() => expect(writes).toEqual([[]]));
});

it("lets a named colleague confirm a link, and shows nothing when none names them", async () => {
  const user = userEvent.setup();
  const answers: unknown[] = [];
  mount(<TeamRequestsCard />, {
    "GET /scheduling/team-requests": () =>
      jsonResponse([
        {
          host_user_id: HOST,
          host_name: "Ada Lovelace",
          title: "Discovery call",
          state: "pending",
        },
      ]),
    [`PUT /scheduling/team-requests/${HOST}`]: (body) => {
      answers.push(body);
      return new Response(null, { status: 204 });
    },
  });
  expect(
    await screen.findByText("Discovery call with Ada Lovelace"),
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(answers).toEqual([{ state: "confirmed" }]));
});

it("renders no card while no link names the colleague", async () => {
  mount(<TeamRequestsCard />, {
    "GET /scheduling/team-requests": () => jsonResponse([]),
  });
  await waitFor(() =>
    expect(screen.queryByText("Colleagues’ links that name you")).toBeNull(),
  );
});
