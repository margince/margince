/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { dayStartIso } from "../design-system/recordtimeline";
import { writeMessage } from "../design-system/richtext-testing";
import { pickOption } from "../design-system/select-testing";
import { calendarDay } from "../format/calendarday";
import { UTC_ZONE } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { RecordZoneProvider } from "../lib/recordzone";
import { meFixture } from "../testing/mefixture";
import { LogActivity } from "./logactivity";
import { appendedNotes } from "./logactivity.openmeeting";

// A meeting logged on a record where a booked meeting owes its outcome is
// asked about. Yes writes the notes and the outcome on that meeting. No logs a
// separate one.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ZONE = UTC_ZONE;

type Captured = {
  key: string;
  body: unknown;
  query: URLSearchParams;
  ifMatch: string | null;
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function booked(over: Record<string, unknown> = {}) {
  return {
    id: "m-1",
    kind: "meeting",
    subject: "Quarterly review",
    body: "Agenda: pricing",
    occurred_at: new Date(Date.now() - 2 * 3_600_000).toISOString(),
    occurred_precision: "instant",
    meeting_status: "booked",
    awaiting_outcome: true,
    writable: true,
    calendar_managed: false,
    version: 4,
    offers_email: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-07-06T08:00:00Z",
    updated_at: "2026-07-06T08:00:00Z",
    ...over,
  };
}

function stubApi(
  meetings: unknown[],
  captured: Captured[],
  patch: () => Response = () =>
    jsonResponse(booked({ meeting_status: "held" })),
) {
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
      let body: unknown = null;
      if (method !== "GET") {
        body = request ? await request.json() : JSON.parse(String(init?.body));
      }
      const ifMatch = request?.headers.get("If-Match") ?? null;
      captured.push({ key, body, query: url.searchParams, ifMatch });
      if (key === "GET /me") {
        return jsonResponse(
          meFixture({ allow: { activity: ["create"], contact: ["read"] } }),
        );
      }
      if (
        key === "GET /activities" &&
        url.searchParams.get("kind") === "meeting"
      ) {
        return jsonResponse({ data: meetings, page: { next_cursor: null } });
      }
      if (key === "PATCH /activities/m-1") return patch();
      if (key === "POST /activities") {
        return jsonResponse(
          { ...booked(), id: "a-new", kind: (body as { kind: string }).kind },
          201,
        );
      }
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }),
  );
}

function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordZoneProvider zone={ZONE}>
          <LogActivity entityType="contact" entityId="p1" />
        </RecordZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const QUESTION = /Is this the record of Quarterly review/;

async function logMeeting(answerNo = false) {
  const user = userEvent.setup();
  await pickOption(user, await screen.findByLabelText("Type"), "Meeting");
  await user.type(screen.getByLabelText("Subject *"), "Review notes");
  writeMessage("Details", "They agreed to the renewal.");
  if (answerNo) {
    await user.click(
      await screen.findByRole("radio", { name: en["log.recordOfMeetingNo"] }),
    );
  }
  return user;
}

const sent = (captured: Captured[], key: string) =>
  captured.filter((entry) => entry.key === key);

describe("logging a meeting where one owes its outcome", () => {
  it("Yes records the notes on the open meeting", async () => {
    const captured: Captured[] = [];
    stubApi([booked()], captured);
    render();
    const user = await logMeeting();
    expect(await screen.findByText(QUESTION)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(sent(captured, "PATCH /activities/m-1")).toHaveLength(1),
    );
    const [patch] = sent(captured, "PATCH /activities/m-1");
    expect(patch?.ifMatch).toBe("4");
    expect(patch?.body).toEqual({
      meeting_status: "held",
      body: "Agenda: pricing\n\nThey agreed to the renewal.",
    });
    expect(sent(captured, "POST /activities")).toHaveLength(0);
  });

  it("No logs a separate held meeting", async () => {
    const captured: Captured[] = [];
    stubApi([booked()], captured);
    render();
    const user = await logMeeting(true);
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(sent(captured, "POST /activities")).toHaveLength(1),
    );
    expect(sent(captured, "POST /activities")[0]?.body).toMatchObject({
      kind: "meeting",
      meeting_status: "held",
    });
    expect(sent(captured, "PATCH /activities/m-1")).toHaveLength(0);
  });

  it("No open meeting asks nothing", async () => {
    const captured: Captured[] = [];
    stubApi([booked({ awaiting_outcome: false })], captured);
    render();
    const user = await logMeeting();
    await waitFor(() =>
      expect(
        captured.some((entry) => entry.query.get("kind") === "meeting"),
      ).toBe(true),
    );
    expect(screen.queryByText(QUESTION)).toBeNull();
    await user.click(screen.getByRole("button", { name: "Log" }));
    await waitFor(() =>
      expect(sent(captured, "POST /activities")).toHaveLength(1),
    );
  });

  it("A calendar-managed meeting gets a linked note and only its status", async () => {
    const captured: Captured[] = [];
    stubApi([booked({ calendar_managed: true })], captured);
    render();
    const user = await logMeeting();
    expect(
      await screen.findByText(en["log.recordOfMeetingManaged"]),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(sent(captured, "PATCH /activities/m-1")).toHaveLength(1),
    );
    expect(sent(captured, "POST /activities")[0]?.body).toMatchObject({
      kind: "note",
      body: "They agreed to the renewal.",
      links: [{ entity_type: "contact", entity_id: "p1" }],
    });
    expect(sent(captured, "PATCH /activities/m-1")[0]?.body).toEqual({
      meeting_status: "held",
    });
  });

  it("The lookup asks a three-day window around the chosen day", async () => {
    const captured: Captured[] = [];
    stubApi([], captured);
    render();
    await logMeeting();
    await waitFor(() =>
      expect(
        captured.some((entry) => entry.query.get("kind") === "meeting"),
      ).toBe(true),
    );
    const day = calendarDay(new Date(), ZONE);
    const lookup = captured.find(
      (entry) => entry.query.get("kind") === "meeting",
    );
    expect(lookup?.query.get("entity_id")).toBe("p1");
    expect(lookup?.query.get("occurred_after")).toBe(
      dayStartIso(day, ZONE, -1),
    );
    expect(lookup?.query.get("occurred_before")).toBe(
      dayStartIso(day, ZONE, 2),
    );
  });

  it("A refused version keeps the draft and logs nothing", async () => {
    const captured: Captured[] = [];
    stubApi([booked()], captured, () =>
      jsonResponse(
        { title: "Precondition Failed", status: 412, code: "version_skew" },
        412,
      ),
    );
    render();
    const user = await logMeeting();
    await screen.findByText(QUESTION);
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(sent(captured, "PATCH /activities/m-1")).toHaveLength(1),
    );
    await waitFor(() =>
      expect(screen.getByLabelText<HTMLInputElement>("Subject *").value).toBe(
        "Review notes",
      ),
    );
    expect(sent(captured, "POST /activities")).toHaveLength(0);
  });
});

describe("appendedNotes", () => {
  it("keeps the meeting's own notes and adds the typed ones after a blank line", () => {
    expect(appendedNotes("Agenda", " Notes ")).toBe("Agenda\n\nNotes");
    expect(appendedNotes(null, "Notes")).toBe("Notes");
    expect(appendedNotes("Agenda", "  ")).toBe("Agenda");
  });
});
