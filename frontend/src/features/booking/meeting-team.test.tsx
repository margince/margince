/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { TeamRequestsCard } from "./meeting-team";

const HOST = "0198f011-bbbb-7000-8000-000000000003";
const PAGE = "0198f011-bbbb-7000-8000-000000000004";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(node: React.ReactNode, routes: RouteMap) {
  installFetchStub(routes);
  render(<StoryProviders>{node}</StoryProviders>);
}

it("lets a named colleague confirm the team link that names them", async () => {
  const user = userEvent.setup();
  const answers: unknown[] = [];
  mount(<TeamRequestsCard />, {
    "GET /scheduling/team-requests": () =>
      jsonResponse([
        {
          page_id: PAGE,
          host_user_id: HOST,
          host_name: "Ada Lovelace",
          title: "Discovery call",
          state: "pending",
        },
      ]),
    [`PUT /scheduling/team-requests/${PAGE}`]: (body) => {
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
