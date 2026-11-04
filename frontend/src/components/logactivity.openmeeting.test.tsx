/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  renderHook,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { isISODate } from "../design-system/dateinput";
import { dayStartIso } from "../design-system/recordtimeline";
import { writeMessage } from "../design-system/richtext-testing";
import { pickOption } from "../design-system/select-testing";
import { calendarDay } from "../format/calendarday";
import { UTC_ZONE } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ViewerZoneProvider } from "../lib/viewerzone";
import { meFixture } from "../testing/mefixture";
import { LogActivity } from "./logactivity";
import {
  appendedNotes,
  closestMeeting,
  useOpenMeetingOnDay,
} from "./logactivity.openmeeting";

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
  idempotencyKey: string | null;
  form: FormData | null;
};

// The reader meFixture signs in.
const ME = meFixture({}).user.id;

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
    host_user_id: ME,
    colleague_ids: [],
    links: [{ entity_type: "contact", entity_id: "p1" }],
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
  post: (body: { kind: string }) => Response | undefined = () => undefined,
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
      const form = init?.body instanceof FormData ? init.body : null;
      let body: unknown = null;
      if (method !== "GET" && !form) {
        body = request ? await request.json() : JSON.parse(String(init?.body));
      }
      const headers = request?.headers;
      captured.push({
        key,
        body,
        query: url.searchParams,
        ifMatch: headers?.get("If-Match") ?? null,
        idempotencyKey: headers?.get("Idempotency-Key") ?? null,
        form,
      });
      if (key === "GET /me") {
        return jsonResponse(
          meFixture({
            allow: { activity: ["create", "update"], contact: ["read"] },
          }),
        );
      }
      if (key === "GET /installation/settings") {
        return jsonResponse({ max_upload_bytes: 10_000_000 });
      }
      if (key === "POST /attachments") {
        return jsonResponse({ id: "att-1" }, 201);
      }
      if (
        key === "GET /activities" &&
        url.searchParams.get("kind") === "meeting"
      ) {
        return jsonResponse({ data: meetings, page: { next_cursor: null } });
      }
      if (key === "PATCH /activities/m-1") return patch();
      if (key === "POST /activities") {
        const kind = (body as { kind: string }).kind;
        return (
          post(body as { kind: string }) ??
          jsonResponse({ ...booked(), id: `a-${kind}`, kind }, 201)
        );
      }
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }),
  );
}

// A refusal the step cannot take back, answered once and then granted.
function failOnce(kind: string) {
  let failed = false;
  return (body: { kind: string }) => {
    if (body.kind !== kind || failed) return undefined;
    failed = true;
    return jsonResponse(
      { title: "Unavailable", status: 503, code: "service_unavailable" },
      503,
    );
  };
}

function render(zone = ZONE) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ViewerZoneProvider zone={zone}>
          <LogActivity entityType="contact" entityId="p1" />
        </ViewerZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const QUESTION = /Is this the record of Quarterly review/;

async function addFollowUp() {
  const user = userEvent.setup();
  await user.click(
    screen.getByRole("checkbox", { name: "Add a follow-up task" }),
  );
  fireEvent.change(screen.getByRole("textbox", { name: "Next step" }), {
    target: { value: "Send revised quote" },
  });
}

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

  it("A calendar-managed meeting gets its status first, then a linked note", async () => {
    const captured: Captured[] = [];
    stubApi([booked({ calendar_managed: true })], captured);
    render();
    const user = await logMeeting();
    expect(
      await screen.findByText(en["log.recordOfMeetingManaged"]),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(sent(captured, "POST /activities")).toHaveLength(1),
    );
    const writes = captured.filter(
      (entry) => entry.key !== "GET /me" && !entry.key.startsWith("GET"),
    );
    expect(writes.map((entry) => entry.key)).toEqual([
      "PATCH /activities/m-1",
      "POST /activities",
    ]);
    expect(sent(captured, "PATCH /activities/m-1")[0]?.body).toEqual({
      meeting_status: "held",
    });
    const [note] = sent(captured, "POST /activities");
    expect(note?.body).toMatchObject({
      kind: "note",
      body: "They agreed to the renewal.",
      links: [{ entity_type: "contact", entity_id: "p1" }],
    });
    expect(note?.idempotencyKey).toMatch(/:note$/);
  });

  it("A calendar-managed follow-up is a task whose source is the meeting", async () => {
    const captured: Captured[] = [];
    stubApi([booked({ calendar_managed: true })], captured);
    render();
    const user = await logMeeting();
    await screen.findByText(QUESTION);
    await addFollowUp();
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(sent(captured, "POST /activities")).toHaveLength(2),
    );
    const [note, task] = sent(captured, "POST /activities");
    expect(note?.body).not.toHaveProperty("follow_up_task");
    expect(task?.body).toMatchObject({
      kind: "task",
      subject: "Send revised quote",
      source_activity_id: "m-1",
    });
    expect(task?.idempotencyKey).toMatch(/:follow-up$/);
    expect(task?.idempotencyKey).not.toBe(note?.idempotencyKey);
  });

  it("A calendar-managed meeting takes the picked files, not the note", async () => {
    const captured: Captured[] = [];
    stubApi([booked({ calendar_managed: true })], captured);
    render();
    const user = await logMeeting();
    await screen.findByText(QUESTION);
    await user.upload(
      await screen.findByLabelText("Attach files"),
      new File(["%PDF-1.4"], "agenda.pdf", { type: "application/pdf" }),
    );
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(sent(captured, "POST /attachments")).toHaveLength(1),
    );
    const [upload] = sent(captured, "POST /attachments");
    expect(upload?.form?.get("entity_type")).toBe("activity");
    expect(upload?.form?.get("entity_id")).toBe("m-1");
  });

  it("A note that fails after the status is written is the only thing a retry sends", async () => {
    const captured: Captured[] = [];
    stubApi(
      [booked({ calendar_managed: true })],
      captured,
      undefined,
      failOnce("note"),
    );
    render();
    const user = await logMeeting();
    await screen.findByText(QUESTION);
    await user.click(screen.getByRole("button", { name: "Log" }));
    expect(
      await screen.findByText(/Quarterly review is already recorded as held/),
    ).toBeTruthy();
    expect(screen.queryByText(QUESTION)).toBeNull();

    await user.click(screen.getByRole("button", { name: "Log" }));
    await waitFor(() =>
      expect(screen.getByLabelText<HTMLInputElement>("Subject *").value).toBe(
        "",
      ),
    );
    expect(sent(captured, "PATCH /activities/m-1")).toHaveLength(1);
    const [first, retry] = sent(captured, "POST /activities");
    expect(retry?.idempotencyKey).toBe(first?.idempotencyKey);
    expect(retry?.body).toEqual(first?.body);
    expect(retry?.body).toMatchObject({ kind: "note" });
  });

  it("A follow-up that fails after the outcome is the only thing a retry sends", async () => {
    const captured: Captured[] = [];
    stubApi([booked()], captured, undefined, failOnce("task"));
    render();
    const user = await logMeeting();
    await screen.findByText(QUESTION);
    await addFollowUp();
    await user.click(screen.getByRole("button", { name: "Log" }));
    expect(
      await screen.findByText(/Quarterly review is already recorded as held/),
    ).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Log" }));
    await waitFor(() =>
      expect(sent(captured, "POST /activities")).toHaveLength(2),
    );
    expect(sent(captured, "PATCH /activities/m-1")).toHaveLength(1);
    const [first, retry] = sent(captured, "POST /activities");
    expect(retry?.body).toMatchObject({
      kind: "task",
      source_activity_id: "m-1",
    });
    expect(retry?.body).toEqual(first?.body);
    expect(retry?.idempotencyKey).toBe(first?.idempotencyKey);
  });

  it("A colleague's meeting is not offered", async () => {
    const captured: Captured[] = [];
    stubApi([booked({ host_user_id: "u-colleague" })], captured);
    render();
    await logMeeting();
    await waitFor(() =>
      expect(
        captured.some((entry) => entry.query.get("kind") === "meeting"),
      ).toBe(true),
    );
    await waitFor(() =>
      expect(sent(captured, "GET /me").length).toBeGreaterThan(0),
    );
    expect(screen.queryByText(QUESTION)).toBeNull();
  });

  it("A meeting the reader attends as a colleague is offered", async () => {
    const captured: Captured[] = [];
    stubApi(
      [booked({ host_user_id: "u-colleague", colleague_ids: [ME] })],
      captured,
    );
    render();
    await logMeeting();
    expect(await screen.findByText(QUESTION)).toBeTruthy();
  });

  it("A meeting not with this contact is not offered", async () => {
    const captured: Captured[] = [];
    stubApi(
      [booked({ links: [{ entity_type: "contact", entity_id: "p-other" }] })],
      captured,
    );
    render();
    await logMeeting();
    await waitFor(() =>
      expect(
        captured.some((entry) => entry.query.get("kind") === "meeting"),
      ).toBe(true),
    );
    expect(screen.queryByText(QUESTION)).toBeNull();
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
    if (!isISODate(day)) throw new Error(`today is not a day: ${day}`);
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

describe("the open meeting on a company", () => {
  function wrapper({ children }: Readonly<{ children: ReactNode }>) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  }
  const alice = booked({
    id: "m-alice",
    links: [{ entity_type: "contact", entity_id: "alice" }],
  });
  const bob = booked({
    id: "m-bob",
    occurred_at: new Date(Date.now() - 3_600_000).toISOString(),
    links: [{ entity_type: "contact", entity_id: "bob" }],
  });
  const today = calendarDay(new Date(), ZONE);
  if (!isISODate(today)) throw new Error(`today is not a day: ${today}`);

  it("offers only the meeting with the attendee the entry is about", async () => {
    stubApi([alice, bob], []);
    const { result } = renderHook(
      () =>
        useOpenMeetingOnDay("company", "c1", today, ZONE, true, {
          attendeeIds: ["alice"],
          viewerId: ME,
        }),
      { wrapper },
    );
    await waitFor(() => expect(result.current?.id).toBe("m-alice"));
  });

  it("offers nothing on a company before an attendee is picked", async () => {
    const captured: Captured[] = [];
    stubApi([alice, bob], captured);
    const { result } = renderHook(
      () =>
        useOpenMeetingOnDay("company", "c1", today, ZONE, true, {
          attendeeIds: [],
          viewerId: ME,
        }),
      { wrapper },
    );
    await waitFor(() =>
      expect(
        captured.some((entry) => entry.query.get("kind") === "meeting"),
      ).toBe(true),
    );
    expect(result.current).toBeUndefined();
  });
});

describe("closestMeeting", () => {
  it("compares a date-only meeting by its day in the reader's zone", () => {
    const zone = "Asia/Ho_Chi_Minh";
    const onTheDay = {
      id: "date-only",
      occurred_at: "2026-07-06T12:00:00Z",
      occurred_precision: "day" as const,
    };
    // 22:00 the evening before, in Ho Chi Minh City: two hours before the
    // day starts, where the date-only meeting's noon UTC is nineteen after.
    const eveningBefore = {
      id: "evening-before",
      occurred_at: "2026-07-05T15:00:00Z",
      occurred_precision: "instant" as const,
    };
    expect(
      closestMeeting([eveningBefore, onTheDay], "2026-07-06", zone)?.id,
    ).toBe("date-only");
  });

  it("breaks a tie on the same day by the instant", () => {
    const early = {
      id: "early",
      occurred_at: "2026-07-06T08:00:00Z",
      occurred_precision: "instant" as const,
    };
    const late = {
      id: "late",
      occurred_at: "2026-07-06T20:00:00Z",
      occurred_precision: "instant" as const,
    };
    expect(closestMeeting([late, early], "2026-07-06", ZONE)?.id).toBe("early");
  });
});

describe("appendedNotes", () => {
  it("keeps the meeting's own notes and adds the typed ones after a blank line", () => {
    expect(appendedNotes("Agenda", " Notes ")).toBe("Agenda\n\nNotes");
    expect(appendedNotes(null, "Notes")).toBe("Notes");
    expect(appendedNotes("Agenda", "  ")).toBe("Agenda");
  });
});
