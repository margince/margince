/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { CaptureHealthCard, sweepWarning } from "./capturehealth";
import { jsonResponse, render } from "./settings.testkit";

type Health = components["schemas"]["CaptureHealth"];
type Sweep = components["schemas"]["CaptureSweepHealth"];

// Every request the card makes, recorded: the withheld case is about the
// ABSENCE of one.
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
      sent.push(key);
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /admin/capture-health") return jsonResponse(HEALTH);
      if (key === "GET /me")
        return jsonResponse(
          meFixture({ roles: ["admin"], allow: { job_health: ["read"] } }),
        );
      return jsonResponse({});
    }),
  );
  return sent;
}

const GENERATED = "2026-09-17T09:30:00Z";

const HEALTH: Health = {
  generated_at: GENERATED,
  mailboxes: [
    {
      user_id: "00000000-0000-7000-8000-000000000001",
      display_name: "Ana Rep",
      contacts_awaiting_decision: 3,
      oldest_contact_age_seconds: 3 * 86_400,
      threads_awaiting_verdict: 1,
      oldest_thread_age_seconds: 7_200,
    },
  ],
  classifier: { pending: 4, unsure: 2, exhausted: 1 },
  held_meetings: { count: 201, oldest_age_seconds: 2 * 86_400 },
  sweeps: [
    {
      sweep: "settled_thread_verdicts",
      cadence_seconds: 600,
      last_succeeded_at: "2026-09-17T09:20:00Z",
      last_run: {
        outcome: "ok",
        finished_at: "2026-09-17T09:20:00Z",
        processed: 2,
        cap_hit: false,
      },
    },
    {
      sweep: "stranded_contacts",
      cadence_seconds: 86_400,
      last_succeeded_at: "2026-09-15T03:00:00Z",
      last_run: {
        outcome: "failed",
        finished_at: "2026-09-17T03:00:00Z",
        processed: 7,
        cap_hit: false,
        error_class: "write_conflict",
      },
    },
    {
      sweep: "filed_meeting_holds",
      cadence_seconds: 86_400,
      last_succeeded_at: "2026-09-17T03:00:00Z",
      last_run: {
        outcome: "partial",
        finished_at: "2026-09-17T03:00:00Z",
        processed: 200,
        cap_hit: true,
      },
    },
  ],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function sweepRow(label: RegExp): HTMLElement {
  const term = screen.getByText(label);
  const row = term.closest(".factlist-row");
  if (!(row instanceof HTMLElement)) {
    throw new Error(`no fact row for ${label}`);
  }
  return row;
}

it("counts each mailbox and the installation without naming what waits", async () => {
  stubRoutes();

  render(<CaptureHealthCard />);

  expect(await screen.findByText("Ana Rep")).toBeInTheDocument();
  expect(screen.getByText("3 contacts waiting")).toBeInTheDocument();
  expect(screen.getByText("1 thread waiting")).toBeInTheDocument();
  expect(
    screen.getByText(/contacts: oldest has waited 3 days/),
  ).toBeInTheDocument();
  expect(screen.getByText("4 pending")).toBeInTheDocument();
  expect(screen.getByText("1 sender with no retries left")).toBeInTheDocument();
  expect(screen.getByText("201 meetings held")).toBeInTheDocument();
});

it("says which pass keeps up, which failed and which left a backlog", async () => {
  stubRoutes();

  render(<CaptureHealthCard />);

  await screen.findByText("Ana Rep");
  expect(
    within(sweepRow(/Settled threads/)).getByText("Keeping up"),
  ).toBeInTheDocument();
  const failed = sweepRow(/Unasked contacts/);
  expect(within(failed).getByText("Last run failed")).toBeInTheDocument();
  // The class verbatim: the token an operator greps the worker log for.
  expect(within(failed).getByText("write_conflict")).toBeInTheDocument();
  expect(
    within(sweepRow(/Filed meetings/)).getByText("Backlog remains"),
  ).toBeInTheDocument();
});

it("reads a calm installation as calm, and still shows how each pass stands", async () => {
  stubRoutes({
    "GET /admin/capture-health": () =>
      jsonResponse({
        ...HEALTH,
        mailboxes: [],
        held_meetings: { count: 0 },
      }),
  });

  render(<CaptureHealthCard />);

  expect(
    await screen.findByText("No mailbox has anything waiting."),
  ).toBeInTheDocument();
  expect(screen.getByText("0 meetings held")).toBeInTheDocument();
  expect(
    within(sweepRow(/Unasked contacts/)).getByText("Last run failed"),
  ).toBeInTheDocument();
});

it("says a pass that never ran has never run, rather than drawing it clean", async () => {
  stubRoutes({
    "GET /admin/capture-health": () =>
      jsonResponse({
        ...HEALTH,
        sweeps: [{ sweep: "stranded_contacts", cadence_seconds: 86_400 }],
      }),
  });

  render(<CaptureHealthCard />);

  const row = await screen.findByText(/Unasked contacts/);
  const fact = row.closest(".factlist-row");
  expect(fact).not.toBeNull();
  expect(
    within(fact as HTMLElement).getByText("Never run"),
  ).toBeInTheDocument();
  expect(
    within(fact as HTMLElement).getByText(/has not succeeded yet/),
  ).toBeInTheDocument();
});

// One part missing at a time, so each part of the shape check is proven on
// its own rather than by a payload that fails several of them at once.
it.each(["generated_at", "mailboxes", "sweeps", "classifier", "held_meetings"])(
  "reports a payload without %s rather than drawing a clean installation",
  async (field) => {
    const { [field as keyof Health]: _dropped, ...partial } = HEALTH;
    stubRoutes({
      "GET /admin/capture-health": () => jsonResponse(partial),
    });

    render(<CaptureHealthCard />);

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.queryByText("Keeping up")).not.toBeInTheDocument();
    // The report refused as unreadable, not a body that crashed on the gap:
    // the boundary's fallback would also be an alert.
    expect(
      screen.queryByText("This card stopped working"),
    ).not.toBeInTheDocument();
  },
);

it("withholds the card from a seat without the grant, and asks the server nothing", async () => {
  const sent = stubRoutes({
    "GET /me": () => jsonResponse(meFixture({ roles: ["rep"] })),
  });

  render(<CaptureHealthCard />);

  expect(
    await screen.findByText(/requires a permission your role does not have/),
  ).toBeInTheDocument();
  expect(sent).not.toContain("GET /admin/capture-health");
});

function sweep(overrides: Partial<Sweep>): Sweep {
  return {
    sweep: "filed_meeting_holds",
    cadence_seconds: 86_400,
    last_succeeded_at: "2026-09-17T03:00:00Z",
    last_run: {
      outcome: "ok",
      finished_at: "2026-09-17T03:00:00Z",
      processed: 0,
      cap_hit: false,
    },
    ...overrides,
  };
}

it("calls a pass overdue once its last success is two cadences behind the report", () => {
  // A day and a half behind a daily pass is one missed night at most.
  expect(
    sweepWarning(
      sweep({ last_succeeded_at: "2026-09-15T22:00:00Z" }),
      GENERATED,
    ),
  ).toBeNull();
  expect(
    sweepWarning(
      sweep({ last_succeeded_at: "2026-09-15T09:00:00Z" }),
      GENERATED,
    )?.label,
  ).toBe("captureHealth.warn.overdue");
  // Skipped says more than overdue: an earlier stage is what stopped it.
  expect(
    sweepWarning(
      sweep({
        last_succeeded_at: "2026-09-10T03:00:00Z",
        last_run: {
          outcome: "skipped",
          finished_at: "2026-09-17T03:00:00Z",
          processed: 0,
          cap_hit: false,
        },
      }),
      GENERATED,
    )?.label,
  ).toBe("captureHealth.warn.skipped");
});
