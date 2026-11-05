/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { jsonResponse, render } from "./users-admin.testkit";
import { MemberTeamsField } from "./users-teams";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// A workspace whose /teams starts empty and grows by what POST /teams makes.
function teamsServer() {
  const teams: { id: string; name: string }[] = [];
  const created: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.includes("/teams") && req.method === "POST") {
        const body = (await req.json()) as { name: string };
        created.push(body);
        const team = { id: `t-${teams.length + 1}`, name: body.name };
        teams.push(team);
        return jsonResponse(team, 201);
      }
      if (req.url.includes("/teams")) {
        return jsonResponse({ data: [...teams], page: { has_more: false } });
      }
      return jsonResponse({}, 404);
    }),
  );
  return created;
}

function Field() {
  const [value, setValue] = useState<string[]>([]);
  return <MemberTeamsField value={value} onChange={setValue} canManage />;
}

it("starts an empty workspace's first team from the field, and puts the member on it", async () => {
  const user = userEvent.setup();
  const created = teamsServer();
  render(<Field />);
  expect(await screen.findByText("No teams yet.")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: /New team/ }));
  await user.type(screen.getByRole("searchbox"), "DACH Sales");
  await user.click(
    screen.getByRole("button", { name: "Create team “DACH Sales”" }),
  );
  const group = screen.getByRole("group", { name: "Teams" });
  await waitFor(() =>
    expect(within(group).getByRole("listitem").textContent).toBe("DACH Sales"),
  );
  expect(created).toEqual([{ name: "DACH Sales" }]);
});
