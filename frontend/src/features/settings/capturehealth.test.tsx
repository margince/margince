/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { translate } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { tableRowCells } from "../../testing/tablerow.testkit";
import { CaptureHealthCard, sweepWarning } from "./capturehealth";

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
  connection_concerns: [],
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

function factRow(label: string): HTMLElement {
  const row = screen.getByText(label).closest(".factlist-row");
  if (!(row instanceof HTMLElement)) {
    throw new Error(`no fact row for ${label}`);
  }
  return row;
}

it("counts each mailbox and the installation without naming what waits", async () => {
  stubRoutes();

  render(<CaptureHealthCard />);

  const box = await tableRowCells("Mailboxes", "Ana Rep");
  expect(box["Contacts waiting"]).toHaveTextContent("3");
  expect(box["Threads waiting"]).toHaveTextContent("1");
  // A count above zero is the one figure in the row worth finding.
  expect(
    within(box["Contacts waiting"]).getByText("3").closest(".badge"),
  ).not.toBeNull();
  // The older of the two waits, in its largest whole unit.
  expect(box["Oldest wait"]).toHaveTextContent(/^3 days/);
  expect(
    within(factRow("Senders waiting for the check")).getByText("4"),
  ).toBeInTheDocument();
  expect(
    within(factRow("Senders with no retries left"))
      .getByText("1")
      .closest(".badge"),
  ).not.toBeNull();
  expect(
    within(factRow("Held meetings")).getByText("201").closest(".badge"),
  ).not.toBeNull();
});

it("says which pass failed and which left a backlog, and leaves a pass that keeps up quiet", async () => {
  stubRoutes();

  render(<CaptureHealthCard />);

  const settled = await tableRowCells("Repair passes", "Settled threads");
  // Keeping up is the normal reading, so it carries no badge.
  expect(settled.State).toBeEmptyDOMElement();
  expect(settled["Runs every"]).toHaveTextContent("10 minutes");
  const failed = await tableRowCells("Repair passes", "Unasked contacts");
  expect(within(failed.State).getByText("Last run failed")).toBeInTheDocument();
  expect(within(failed.State).getByText("write_conflict")).toBeInTheDocument();
  const filed = await tableRowCells("Repair passes", "Filed meetings");
  expect(within(filed.State).getByText("Backlog remains")).toBeInTheDocument();
  expect(filed.Processed).toHaveTextContent("200");
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

  // The count says no mailbox waits, so no empty mailbox table is drawn.
  const mailboxes = translate("en", "captureHealth.fact.mailboxes");
  await screen.findByText(mailboxes);
  expect(within(factRow(mailboxes)).getByText("0")).toBeInTheDocument();
  expect(
    screen.queryByRole("table", { name: "Mailboxes" }),
  ).not.toBeInTheDocument();
  expect(
    within(factRow("Held meetings")).getByText("0").closest(".badge"),
  ).toBeNull();
  const failed = await tableRowCells("Repair passes", "Unasked contacts");
  expect(within(failed.State).getByText("Last run failed")).toBeInTheDocument();
});

it("leaves out the state column while every pass keeps up", async () => {
  stubRoutes({
    "GET /admin/capture-health": () =>
      jsonResponse({ ...HEALTH, sweeps: [HEALTH.sweeps[0]] }),
  });

  render(<CaptureHealthCard />);

  const settled = await tableRowCells("Repair passes", "Settled threads");
  expect(settled["Runs every"]).toHaveTextContent("10 minutes");
  expect(settled.State).toBeUndefined();
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

  const pass = await tableRowCells("Repair passes", "Unasked contacts");
  expect(within(pass.State).getByText("Never run")).toBeInTheDocument();
  expect(pass["Last success"]).toHaveTextContent("Never");
});

it.each(["generated_at", "mailboxes", "sweeps", "classifier", "held_meetings"])(
  "reports a payload without %s rather than drawing a clean installation",
  async (field) => {
    const { [field as keyof Health]: _dropped, ...partial } = HEALTH;
    stubRoutes({
      "GET /admin/capture-health": () => jsonResponse(partial),
    });

    render(<CaptureHealthCard />);

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(
      screen.queryByRole("table", { name: "Repair passes" }),
    ).not.toBeInTheDocument();
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

it("lists concerned connections without message details", async () => {
  stubRoutes({
    "GET /admin/capture-health": () =>
      jsonResponse({
        ...HEALTH,
        connection_concerns: [
          {
            user_id: "00000000-0000-7000-8000-000000000001",
            connection_id: "00000000-0000-7000-8000-000000000002",
            display_name: "Demo Member",
            provider: "gmail",
            account_label: "mailbox@example.test",
            concern: "sync_stalled",
            since: "2026-09-15T09:30:00Z",
            last_success_at: "2026-09-15T09:30:00Z",
          },
        ],
      }),
  });
  render(<CaptureHealthCard />);
  expect(await screen.findByText("mailbox@example.test")).toBeTruthy();
  expect(screen.getByText("Sync has stopped")).toBeTruthy();
});
