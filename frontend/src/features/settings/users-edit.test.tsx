// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
/** @vitest-environment happy-dom */
import { UTC_ZONE } from "../../format/timezone";
import { backend, render } from "./users-admin.testkit";
import { EditMember } from "./users-edit";
import type { User } from "./users-members";

const MEMBER: User = {
  id: "u-invited",
  display_name: "Ivy Imported",
  email: "ivy@example.test",
  timezone: UTC_ZONE,
  status: "invited",
  email_editable: true,
  is_agent: false,
  roles: ["rep"],
  team_ids: [],
  updated_at: "2026-10-01T09:00:00Z",
  allowed_actions: ["edit", "change_role"],
};
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
it("saves a reactivated invitation's name and team before first sign-in", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  const routed = backend([]);
  const saved: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.method === "PATCH") {
        saved.push(await req.json());
        return new Response(null, { status: 204 });
      }
      return routed(input, init);
    }),
  );
  const closed = vi.fn();
  render(<EditMember member={MEMBER} onClose={closed} />);
  const name = screen.getByRole("textbox", { name: "Full name" });
  await user.clear(name);
  await user.type(name, "Ivy Corrected");
  await user.click(await screen.findByRole("checkbox", { name: "DACH Sales" }));
  await user.click(screen.getByRole("button", { name: "Save member" }));
  await waitFor(() => expect(closed).toHaveBeenCalledOnce());
  expect(saved).toEqual([
    {
      display_name: "Ivy Corrected",
      email: MEMBER.email,
      team_ids: ["t-dach"],
      expected_updated_at: MEMBER.updated_at,
    },
  ]);
});
