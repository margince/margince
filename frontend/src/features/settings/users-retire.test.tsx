// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
/** @vitest-environment happy-dom */
import { UTC_ZONE } from "../../format/timezone";
import { en } from "../../i18n/en";
import { backend, jsonResponse, render } from "./users-admin.testkit";
import type { User } from "./users-members";
import { RetireMember } from "./users-retire";

const SOURCE: User = {
  id: "duplicate",
  display_name: "Imported colleague",
  email: "old@example.test",
  timezone: UTC_ZONE,
  status: "deactivated",
  is_agent: false,
};
const SURVIVOR: User = {
  ...SOURCE,
  id: "survivor",
  email: "current@example.test",
  status: "invited",
};
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
it("shows readable blockers and only submits the selected survivor's fresh preview", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  const routed = backend([]);
  const saved: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(req.url);
      if (url.pathname.endsWith("/retirement-preview")) {
        const selected = url.searchParams.get("to_user_id");
        return jsonResponse({
          source_name: SOURCE.display_name,
          source_email: SOURCE.email,
          survivor_email: selected ? SURVIVOR.email : undefined,
          owned: {
            contact: 0,
            company: 0,
            lead: 0,
            deal: 0,
            project: 0,
            task: 0,
          },
          signal_count: 0,
          blockers: selected ? [] : ["choose_survivor"],
          consequences: ["survivor_inactive"],
          token: selected ? "selected-preview" : "initial-preview",
        });
      }
      if (req.method === "POST" && url.pathname.endsWith("/merge")) {
        saved.push(await req.json());
        return jsonResponse({ merged: true });
      }
      return routed(input, init);
    }),
  );
  const closed = vi.fn();
  render(
    <RetireMember
      member={SOURCE}
      members={[
        SOURCE,
        ...Array.from({ length: 250 }, (_, index) => ({
          ...SURVIVOR,
          id: `other-${index}`,
          display_name: `Colleague ${index}`,
          email: `colleague${index}@example.test`,
        })),
        SURVIVOR,
      ]}
      merge
      onClose={closed}
    />,
  );
  expect(
    await screen.findByText(en["users.retirement.choose_survivor"]),
  ).toBeTruthy();
  expect(
    screen.getByText(en["users.retirement.survivor_inactive"]),
  ).toBeTruthy();
  const confirm = screen.getByRole("button", { name: en["users.merge"] });
  expect(confirm).toHaveProperty("disabled", true);
  await user.type(
    screen.getByRole("searchbox", { name: en["users.survivingMember"] }),
    SURVIVOR.email,
  );
  await user.click(
    await screen.findByRole("button", {
      name: `${SURVIVOR.display_name} ${SURVIVOR.email}`,
    }),
  );
  await waitFor(() => expect(confirm).toHaveProperty("disabled", false));
  await user.click(confirm);
  await waitFor(() => expect(closed).toHaveBeenCalledOnce());
  expect(saved).toEqual([
    { to_user_id: SURVIVOR.id, preview_token: "selected-preview" },
  ]);
});

it("searches removal receivers by name and lets the administrator clear the handover", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  const routed = backend([]);
  const choices: (string | null)[] = [];
  const saved: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(request.url);
      if (url.pathname.endsWith("/retirement-preview")) {
        choices.push(url.searchParams.get("to_user_id"));
        return jsonResponse({
          owned: {
            contact: 0,
            company: 0,
            lead: 0,
            deal: 0,
            project: 0,
            task: 0,
          },
          signal_count: 0,
          blockers: [],
          consequences: [],
          token: "preview",
        });
      }
      if (request.method === "POST" && url.pathname.endsWith("/remove")) {
        saved.push(await request.json());
        return jsonResponse({ removed: true });
      }
      return routed(input, init);
    }),
  );
  render(
    <RetireMember
      member={SOURCE}
      members={[SOURCE, { ...SURVIVOR, status: "active" }]}
      merge={false}
      onClose={() => undefined}
    />,
  );
  await user.type(
    screen.getByRole("searchbox", { name: en["users.handover.action"] }),
    SURVIVOR.display_name,
  );
  await user.click(
    await screen.findByRole("button", {
      name: `${SURVIVOR.display_name} ${SURVIVOR.email}`,
    }),
  );
  expect(
    screen.queryByRole("button", {
      name: `${SOURCE.display_name} ${SOURCE.email}`,
    }),
  ).toBeNull();
  await waitFor(() => expect(choices).toContain(SURVIVOR.id));
  await user.click(screen.getByRole("button", { name: en["field.unset"] }));
  expect(screen.queryByText(SURVIVOR.email)).toBeNull();
  expect(screen.getByText(en["users.noHandover"])).toBeTruthy();
  expect(document.activeElement).toBe(
    screen.getByRole("searchbox", { name: en["users.handover.action"] }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: en["users.remove"] }),
    ).toHaveProperty("disabled", false),
  );
  expect(screen.queryByRole("button", { name: en["field.unset"] })).toBeNull();
  await user.click(screen.getByRole("button", { name: en["users.remove"] }));
  await waitFor(() => expect(saved).toEqual([{ preview_token: "preview" }]));
});
