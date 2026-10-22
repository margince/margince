/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { accountsOf } from "../../format/connectorprovider";
import { LocaleProvider } from "../../i18n";
import type { ImportWindow } from "../../lib/connect-account";
import { locationDouble } from "../../testing/locationdouble";
import { ConnectionAccountRow } from "./connection-account";

// One account on Connections: its services each say how they are doing and
// carry their own verb. The verbs about the whole account sit in its menu.

type CaptureConnection = components["schemas"]["CaptureConnection"];
type BackfillStatus = components["schemas"]["BackfillStatus"];

const gmail: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  can_backfill: true,
  status: "connected",
  account_label: "lars@example.de",
  scopes: [
    "https://www.googleapis.com/auth/gmail.readonly",
    "https://www.googleapis.com/auth/gmail.send",
  ],
  last_synced_at: "2026-07-23T09:30:00Z",
  backfill: { state: "done", window: "6m" },
};

const gcal: CaptureConnection = {
  ...gmail,
  id: "018f3a1b-0000-7000-8000-0000000000ca",
  provider: "gcal",
  can_backfill: false,
  scopes: ["https://www.googleapis.com/auth/calendar.events.owned"],
  backfill: undefined,
};

const running: BackfillStatus = {
  state: "running",
  window: "6m",
  estimated_messages: 3560,
  counts: {
    messages_scanned: 1240,
    captured: 412,
    contacts_created: 38,
    companies_created: 9,
  },
  updated_at: new Date().toISOString(),
};

type Handler = (request: Request) => Response | Promise<Response>;

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Routes match on method and the end of the path. Anything unrouted answers an
// empty page, which is what the settings reads beside the row fall back on.
function stubApi(routes: Record<string, Handler> = {}) {
  const calls: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      calls.push(request);
      const path = new URL(request.url).pathname;
      const route = Object.keys(routes).find((key) => {
        const [method, suffix] = key.split(" ");
        return request.method === method && path.endsWith(suffix);
      });
      return route ? routes[route](request) : jsonResponse({ data: [] });
    }),
  );
  return calls;
}

function requestsTo(calls: Request[], suffix: string, method: string) {
  return calls.filter(
    (r) => new URL(r.url).pathname.endsWith(suffix) && r.method === method,
  );
}

function renderAccount(
  connections: CaptureConnection[],
  heldWindow: ImportWindow | null = null,
) {
  const [account] = accountsOf(connections);
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ConnectionAccountRow
          account={account}
          heldWindow={heldWindow}
          onImapReconnect={() => undefined}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function chooseAccountVerb(verb: string) {
  await userEvent.click(
    await screen.findByRole("button", { name: "More actions for Google" }),
  );
  await userEvent.click(await screen.findByRole("button", { name: verb }));
}

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the past email service", () => {
  it("draws a running import against its exact estimate, with Stop", async () => {
    const calls = stubApi({
      "GET /connectors/gmail/backfill": () => jsonResponse(running),
      "DELETE /connectors/gmail/backfill": () =>
        jsonResponse({ ...running, state: "cancelled" }),
    });
    renderAccount([{ ...gmail, backfill: running }, gcal]);
    const service = await screen.findByTestId("connector-gmail-past-email");

    const meter = within(service).getByRole("meter");
    expect(meter).toHaveAttribute("aria-valuenow", "1240");
    expect(meter).toHaveAttribute("aria-valuemax", "3560");
    expect(
      within(service).getByText("1,240 of about 3,560 emails checked"),
    ).toBeInTheDocument();
    // The run's state word sits in a live region, so its change is spoken.
    expect(within(service).getByRole("status").textContent).toBe("Reading");
    expect(within(service).getByText("412")).toBeInTheDocument();
    expect(screen.getByText("Syncing")).toBeInTheDocument();

    await userEvent.click(
      within(service).getByRole("button", { name: "Stop" }),
    );
    await waitFor(() =>
      expect(requestsTo(calls, "/backfill", "DELETE")).toHaveLength(1),
    );
  });

  // Gmail stops counting a large mailbox and answers a floor; a bar against a
  // floor would claim a share of a total nobody knows.
  it("counts without a bar when the estimate is a floor", async () => {
    const floor = { ...running, estimate_is_floor: true };
    stubApi({ "GET /connectors/gmail/backfill": () => jsonResponse(floor) });
    renderAccount([{ ...gmail, backfill: floor }]);
    const service = await screen.findByTestId("connector-gmail-past-email");

    expect(within(service).queryByRole("meter")).toBeNull();
    expect(
      within(service).getByText("1,240 emails checked"),
    ).toBeInTheDocument();
  });

  it("offers the import without counting it when none was asked for", async () => {
    const calls = stubApi();
    renderAccount([{ ...gmail, backfill: { state: "none" } }]);
    const service = await screen.findByTestId("connector-gmail-past-email");

    expect(within(service).getByText("Not imported")).toBeInTheDocument();
    expect(requestsTo(calls, "/backfill/preview", "POST")).toHaveLength(0);
  });

  // The connect asked for a year; the server held it above its cap, so the
  // row counts that year and waits for one press.
  it("counts a held window and starts it on one press", async () => {
    const calls = stubApi({
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "12m",
          estimated_messages: 8200,
          computed_at: "2026-07-23T10:00:00Z",
        }),
      "POST /connectors/gmail/backfill": () =>
        jsonResponse({ ...running, state: "queued", window: "12m" }, 202),
      "GET /connectors/gmail/backfill": () =>
        jsonResponse({ ...running, state: "queued", window: "12m" }),
    });
    renderAccount([{ ...gmail, backfill: { state: "none" } }], "12m");
    const service = await screen.findByTestId("connector-gmail-past-email");

    expect(within(service).getByText("Past email, 1 year")).toBeInTheDocument();
    expect(
      await within(service).findByText("8,200 emails in that time."),
    ).toBeInTheDocument();
    const [preview] = requestsTo(calls, "/backfill/preview", "POST");
    expect(await preview.clone().json()).toEqual({ window: "12m" });

    await userEvent.click(
      within(service).getByRole("button", { name: "Start import" }),
    );
    await waitFor(() =>
      expect(requestsTo(calls, "/backfill", "POST")).toHaveLength(1),
    );
    const [start] = requestsTo(calls, "/backfill", "POST");
    expect(await start.clone().json()).toEqual({ window: "12m" });
    await waitFor(() =>
      expect(within(service).getByRole("status").textContent).toBe(
        "Starting soon",
      ),
    );
  });

  // A reconnect asked for a wider window than the import that already ran, and
  // the server held it. The finished run must not hide the press it owes.
  it("offers a held window over an import that already finished", async () => {
    const calls = stubApi({
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "12m",
          estimated_messages: 8200,
          computed_at: "2026-07-23T10:00:00Z",
        }),
      "POST /connectors/gmail/backfill": () =>
        jsonResponse({ ...running, state: "queued", window: "12m" }, 202),
    });
    renderAccount([gmail], "12m");
    const service = await screen.findByTestId("connector-gmail-past-email");

    expect(within(service).getByText("Past email, 1 year")).toBeInTheDocument();
    expect(
      await within(service).findByText("8,200 emails in that time."),
    ).toBeInTheDocument();
    await userEvent.click(
      within(service).getByRole("button", { name: "Start import" }),
    );
    await waitFor(() =>
      expect(requestsTo(calls, "/backfill", "POST")).toHaveLength(1),
    );
    const [start] = requestsTo(calls, "/backfill", "POST");
    expect(await start.clone().json()).toEqual({ window: "12m" });
  });

  it("stops offering a held window once a run of it has finished", async () => {
    const calls = stubApi();
    renderAccount(
      [{ ...gmail, backfill: { state: "done", window: "12m" } }],
      "12m",
    );
    const service = await screen.findByTestId("connector-gmail-past-email");

    expect(within(service).getByRole("status").textContent).toBe("Imported");
    expect(
      within(service).queryByRole("button", { name: "Start import" }),
    ).toBeNull();
    expect(requestsTo(calls, "/backfill/preview", "POST")).toHaveLength(0);
  });

  it("opens the import dialog from the menu, where the scope is counted", async () => {
    const calls = stubApi({
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "6m",
          estimated_messages: 400,
          computed_at: "2026-07-23T10:00:00Z",
        }),
    });
    renderAccount([{ ...gmail, backfill: { state: "none" } }]);
    await chooseAccountVerb("Import older email");

    const dialog = await screen.findByRole("dialog", {
      name: "Import older email",
    });
    expect(
      await within(dialog).findByText("400 emails in that time."),
    ).toBeInTheDocument();
    expect(requestsTo(calls, "/backfill/preview", "POST")).toHaveLength(1);
  });
});

describe("each service's health", () => {
  it("marks the broken service once and puts its reconnect on the account", async () => {
    stubApi();
    renderAccount([
      { ...gmail, status: "reauth_required", last_sync_error_class: "auth" },
      gcal,
    ]);
    const newMail = await screen.findByTestId("connector-gmail");
    const calendar = screen.getByTestId("connector-gcal");

    expect(within(newMail).getByText("Needs reconnect")).toBeInTheDocument();
    expect(within(calendar).getByText("Up to date")).toBeInTheDocument();
    for (const service of [newMail, calendar]) {
      expect(
        within(service).queryByRole("button", { name: "Reconnect" }),
      ).toBeNull();
    }
    const reconnect = screen.getByRole("button", { name: "Reconnect" });
    expect(reconnect).toHaveClass("btn-primary");
    expect(screen.getByText("Needs attention")).toBeInTheDocument();
    expect(
      screen.getAllByText(/./, { selector: ".provider-row-reason" }),
    ).toHaveLength(1);
  });

  it("offers no reconnect for a failure streak a reconnect would not mend", async () => {
    stubApi();
    renderAccount([{ ...gmail, last_sync_error_class: "rate_limited" }, gcal]);
    await screen.findByTestId("connector-gmail");

    expect(screen.getAllByText("Needs attention")).toHaveLength(2);
    expect(screen.queryByRole("button", { name: "Reconnect" })).toBeNull();
  });

  it("offers the calendar a mailbox-only account is missing", async () => {
    vi.stubGlobal("location", locationDouble({ assign: vi.fn() }));
    const calls = stubApi({
      "POST /connectors/gcal/connect": () =>
        jsonResponse({ authorize_url: "https://accounts.google/consent" }),
    });
    renderAccount([gmail]);
    const calendar = await screen.findByTestId("connector-calendar-missing");

    await userEvent.click(
      within(calendar).getByRole("button", { name: "Connect calendar" }),
    );
    await waitFor(() =>
      expect(requestsTo(calls, "/gcal/connect", "POST")).toHaveLength(1),
    );
    const [connect] = requestsTo(calls, "/gcal/connect", "POST");
    expect(await connect.clone().json()).toEqual({ return_to: "settings" });
  });
});

describe("the account's settings", () => {
  // The tag is one answer per connection. A mailbox with its calendar has three
  // rarer settings behind Advanced, and the summary says so while shut.
  it("counts what Advanced holds for a mailbox and its calendar", async () => {
    stubApi();
    renderAccount([gmail, gcal]);

    expect(
      await screen.findByRole("heading", {
        name: "Who sees mail from lars@example.de",
      }),
    ).toBeInTheDocument();
    expect(screen.getByText("Advanced")).toBeInTheDocument();
    expect(screen.getByText("4 settings")).toBeInTheDocument();
  });

  it("keeps only the tag under a calendar with no mailbox", async () => {
    stubApi();
    renderAccount([gcal]);

    await screen.findByTestId("connector-gcal");
    expect(screen.queryByText("Advanced")).toBeNull();
    expect(screen.queryByText(/Who sees mail from/)).toBeNull();
  });
});

describe("the account's own verbs", () => {
  it("disconnects every service of the account after one confirm", async () => {
    const calls = stubApi({
      "POST /disconnect": () => new Response(null, { status: 204 }),
    });
    renderAccount([gmail, gcal]);
    await chooseAccountVerb("Disconnect");

    const dialog = await screen.findByRole("dialog", {
      name: "Disconnect lars@example.de?",
    });
    expect(
      within(dialog).getByText(/stops Mail and Calendar/),
    ).toBeInTheDocument();
    expect(requestsTo(calls, "/disconnect", "POST")).toHaveLength(0);
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Disconnect" }),
    );
    await waitFor(() =>
      expect(requestsTo(calls, "/disconnect", "POST")).toHaveLength(2),
    );
    const paths = requestsTo(calls, "/disconnect", "POST").map(
      (r) => new URL(r.url).pathname,
    );
    expect(paths.some((p) => p.endsWith("/gmail/disconnect"))).toBe(true);
    expect(paths.some((p) => p.endsWith("/gcal/disconnect"))).toBe(true);
  });

  it("stops only the service unticked in Change what it brings in", async () => {
    const calls = stubApi({
      "POST /disconnect": () => new Response(null, { status: 204 }),
    });
    renderAccount([gmail, gcal]);
    await chooseAccountVerb("Change what it brings in");

    const dialog = await screen.findByRole("dialog", {
      name: "What lars@example.de brings in",
    });
    await userEvent.click(
      within(dialog).getByRole("checkbox", { name: /Calendar/ }),
    );
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));

    const confirm = await screen.findByRole("dialog", {
      name: "Stop bringing in Calendar?",
    });
    await userEvent.click(
      within(confirm).getByRole("button", { name: "Disconnect" }),
    );
    await waitFor(() =>
      expect(requestsTo(calls, "/disconnect", "POST")).toHaveLength(1),
    );
    const [stop] = requestsTo(calls, "/disconnect", "POST");
    expect(new URL(stop.url).pathname.endsWith("/gcal/disconnect")).toBe(true);
  });

  it("asks the vendor for a service ticked in Change what it brings in", async () => {
    vi.stubGlobal("location", locationDouble({ assign: vi.fn() }));
    const calls = stubApi({
      "POST /connectors/gcal/connect": () =>
        jsonResponse({ authorize_url: "https://accounts.google/consent" }),
    });
    renderAccount([gmail]);
    await chooseAccountVerb("Change what it brings in");

    const dialog = await screen.findByRole("dialog", {
      name: "What lars@example.de brings in",
    });
    await userEvent.click(
      within(dialog).getByRole("checkbox", { name: /Calendar/ }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Continue to Google" }),
    );
    await waitFor(() =>
      expect(requestsTo(calls, "/gcal/connect", "POST")).toHaveLength(1),
    );
    expect(requestsTo(calls, "/disconnect", "POST")).toHaveLength(0);
  });
});
