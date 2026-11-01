/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { AutomaticWorkHealthCard } from "./automaticworkhealth";

const ZERO = {
  created: 0,
  not_mine: 0,
  rerouted: 0,
  dismissed: 0,
  rejected: 0,
  completed: 0,
  expired: 0,
  open: 0,
};

function tally(
  kind: string,
  counts: Partial<typeof ZERO>,
  rest: Record<string, unknown> = {},
) {
  const full = { ...ZERO, ...counts };
  return {
    kind,
    counts: full,
    not_mine_share: full.created > 0 ? full.not_mine / full.created : null,
    small_sample: full.created < 10,
    above_threshold: false,
    by_day: full.created > 0 ? [{ date: "2026-10-20", counts: full }] : [],
    ...rest,
  };
}

const idle = [
  tally("promise_task", {}),
  tally("reply_reminder", {}),
  tally("approval_request", {}, { not_mine_share: null }),
];

const HEALTH = {
  generated_at: "2026-10-22T09:00:00Z",
  days: 7,
  first_day: "2026-10-16",
  last_day: "2026-10-22",
  share_threshold: 0.1,
  small_sample_below: 10,
  kinds: [
    { kind: "promise_task", records_not_mine: true },
    { kind: "reply_reminder", records_not_mine: true },
    { kind: "approval_request", records_not_mine: false },
  ],
  unattributed: 0,
  colleagues: [
    {
      user_id: "00000000-0000-7000-8000-000000000001",
      display_name: "Ada Rep",
      status: "active",
      waiting_not_mine: 2,
      tallies: [
        tally(
          "promise_task",
          { created: 10, not_mine: 2, completed: 5, open: 3 },
          { small_sample: false, above_threshold: true },
        ),
        tally("reply_reminder", { created: 2, not_mine: 1, open: 1 }),
        tally(
          "approval_request",
          { created: 3, rejected: 1, completed: 2 },
          { not_mine_share: null },
        ),
      ],
    },
    {
      user_id: "00000000-0000-7000-8000-000000000002",
      display_name: "Quiet Colleague",
      status: "active",
      waiting_not_mine: 0,
      tallies: idle,
    },
  ],
};

function stubRoutes(overrides: Record<string, () => Response> = {}) {
  const sent: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      sent.push(`${key}${url.search}`);
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /admin/automatic-work-health")
        return jsonResponse(HEALTH);
      if (key === "GET /me")
        return jsonResponse(
          meFixture({ roles: ["admin"], allow: { job_health: ["read"] } }),
        );
      return jsonResponse({});
    }),
  );
  return sent;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("shows each colleague's counts per kind and flags only a share the server flags", async () => {
  stubRoutes();

  render(<AutomaticWorkHealthCard />);

  const table = await screen.findByRole("table", { name: "Automatic work" });
  const promise = within(table)
    .getByText("Task from a commitment")
    .closest("tr");
  expect(promise).not.toBeNull();
  const flagged = within(promise as HTMLElement).getByText("20%, needs a look");
  expect(flagged.closest(".badge")).not.toBeNull();

  // Half of two items is shown, and said to be too few to judge.
  const reminder = within(table).getByText("Reply reminder").closest("tr");
  expect(
    within(reminder as HTMLElement).getByText("50%, too few items to judge"),
  ).toBeInTheDocument();

  // A rejected approval is not a Not mine: its kind has no such choice.
  const approval = within(table).getByText("Approval request").closest("tr");
  expect(
    within(approval as HTMLElement).getByText("No Not mine choice"),
  ).toBeInTheDocument();

  // A colleague with nothing is counted, not drawn as a row of zeros.
  expect(within(table).queryByText("Quiet Colleague")).not.toBeInTheDocument();
  expect(
    screen.getByText("1 colleague had no automatic work in these days."),
  ).toBeInTheDocument();

  const waiting = screen.getByRole("table", {
    name: "Waiting messages marked Not mine",
  });
  expect(within(waiting).getByText("Ada Rep")).toBeInTheDocument();
});

it("asks for thirty days when the reader picks them", async () => {
  const sent = stubRoutes();
  const user = userEvent.setup();

  render(<AutomaticWorkHealthCard />);

  await user.click(await screen.findByRole("button", { name: "Last 30 days" }));

  await vi.waitFor(() =>
    expect(sent).toContain("GET /admin/automatic-work-health?days=30"),
  );
});

it("says so when there was no automatic work at all", async () => {
  stubRoutes({
    "GET /admin/automatic-work-health": () =>
      jsonResponse({
        ...HEALTH,
        colleagues: [{ ...HEALTH.colleagues[1] }],
      }),
  });

  render(<AutomaticWorkHealthCard />);

  expect(
    await screen.findByText(
      "Margince created no automatic work in these days.",
    ),
  ).toBeInTheDocument();
});

it("reads a tally missing a count as an error, not as zero", async () => {
  const broken = structuredClone(HEALTH);
  const counts: Record<string, number> = broken.colleagues[0].tallies[0].counts;
  delete counts.not_mine;
  stubRoutes({
    "GET /admin/automatic-work-health": () => jsonResponse(broken),
  });

  render(<AutomaticWorkHealthCard />);

  expect(await screen.findByRole("alert")).toBeInTheDocument();
});

it("withholds the card from a seat without the grant, and asks the server nothing", async () => {
  const sent = stubRoutes({
    "GET /me": () => jsonResponse(meFixture({ roles: ["rep"] })),
  });

  render(<AutomaticWorkHealthCard />);

  expect(
    await screen.findByText(/needs a permission your role does not have/),
  ).toBeInTheDocument();
  expect(
    sent.some((line) => line.startsWith("GET /admin/automatic-work-health")),
  ).toBe(false);
});

it("hides the previous reader's automatic work while the new scope loads", async () => {
  stubRoutes();
  const { client } = render(<AutomaticWorkHealthCard />);
  await screen.findAllByText("Ada Rep");
  const originalFetch = fetch;
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/admin/automatic-work-health"))
      return new Promise<Response>(() => {});
    return originalFetch(input, init);
  });
  act(() =>
    client.setQueryData(["me"], {
      ...meFixture({ roles: ["admin"], allow: { job_health: ["read"] } }),
      workspace_name: "Another synthetic workspace",
    }),
  );
  await waitFor(() => expect(screen.queryAllByText("Ada Rep")).toHaveLength(0));
});
