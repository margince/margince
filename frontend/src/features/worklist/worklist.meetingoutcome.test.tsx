// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A meeting that already happened, answered from the row that asked.
//
// The row used to offer three verbs and nothing else: each wrote one word and
// closed the card, so the one moment a human knows what came of a meeting was
// the moment the product could be told the least. Now the card carries the two
// answers that differ in KIND — a cancellation, which needs no prose, and an
// update, which opens the composer on this meeting so the outcome is typed
// where the reader is standing.
//
// What these pin is that the update PATCHES the meeting rather than logging a
// second activity beside it. A composer that POSTed would leave the original
// sitting in the queue unanswered and put two rows on the account's timeline
// for one meeting — and every visible check on this screen would still pass.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  messageBox,
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { RecordZoneProvider } from "../../lib/recordzone";
import { day, row, stub } from "../../testing/worklist.testkit";
import { WorklistScreen } from "./worklist";

const MEETING_ID = "01a05500-0000-7000-8000-0000000000b1";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function aMeetingOwedAnAnswer(over = {}) {
  return row({
    id: MEETING_ID,
    source: "meeting_outcome",
    category: "meetings",
    title: "Discovery call with Turbinenbau",
    band: "keep_momentum",
    destination: "today",
    actions: ["decide"],
    ...over,
  });
}

function aDayWithOne(over = {}) {
  return day({
    queue: [aMeetingOwedAnAnswer(over)],
    summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
  });
}

// The meeting as its own endpoint answers it, which is what the dialog reads to
// seed its form. The body is the calendar's own excerpt — the shape
// capture/meetingmap composes — because editing that text without destroying
// the attendee record is the thing the dialog claims to do.
const CAPTURED_MEETING = {
  id: MEETING_ID,
  kind: "meeting",
  subject: "Discovery call with Turbinenbau",
  body: "Organizer: greta@turbinenbau.example\nAttendees: lars@gradion.com",
  // 21:00 in Bangkok and 09:00 in New York, noon in neither zone. A save that
  // wrote the zone's noon cannot pass for one that kept the start.
  occurred_at: "2026-01-15T14:00:00Z",
  occurred_precision: "instant",
  meeting_status: "booked",
  version: 3,
  source: "gcal",
  captured_by: "connector:gcal",
  created_at: "2026-08-31T08:00:00Z",
  updated_at: "2026-08-31T08:00:00Z",
};

// Serves the worklist read, the meeting read behind the dialog, and records
// every PATCH body so a test can assert what the server was actually told.
function stubWithMeetingRead(
  sent: unknown[],
  patchStatus = 200,
  meeting: Record<string, unknown> = CAPTURED_MEETING,
  preconditions: (string | null)[] = [],
) {
  stub(aDayWithOne());
  const passthrough = globalThis.fetch;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : undefined;
      const url = String(request ? request.url : input);
      const method = String(request?.method ?? init?.method ?? "GET");
      if (/\/activities\/[^/]+$/.test(url.split("?")[0])) {
        if (method === "PATCH") {
          // openapi-fetch hands the stub a Request, not (url, init), so the
          // body is read off the request — init is undefined here and would
          // capture nothing.
          const body = request
            ? await request.clone().text()
            : String(init?.body ?? "");
          if (body !== "") {
            sent.push(JSON.parse(body));
          }
          const precondition = request?.headers.get("If-Match") ?? null;
          preconditions.push(precondition);
          if (precondition !== String(meeting.version)) {
            return new Response(
              JSON.stringify({
                title: "Conflict",
                status: 409,
                code: "version_skew",
              }),
              {
                status: 409,
                headers: { "content-type": "application/problem+json" },
              },
            );
          }
          // The server's rule: a live invitation is canceled through itself.
          const live =
            meeting.invitation_status &&
            meeting.invitation_status !== "canceled";
          if (live && body.includes('"canceled"')) {
            return new Response(
              JSON.stringify({
                title: "Unprocessable",
                status: 422,
                code: "calendar_managed",
              }),
              {
                status: 422,
                headers: { "content-type": "application/problem+json" },
              },
            );
          }
          if (patchStatus !== 200) {
            return new Response(JSON.stringify({ title: "no" }), {
              status: patchStatus,
              headers: { "content-type": "application/problem+json" },
            });
          }
          return new Response(JSON.stringify({ ...meeting, version: 4 }), {
            status: 200,
            headers: { "content-type": "application/json" },
          });
        }
        return new Response(JSON.stringify(meeting), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      return passthrough(input, init);
    }),
  );
}

describe("a meeting that owes an answer", () => {
  it("offers the two answers that differ in kind, not three statuses", async () => {
    stub(aDayWithOne());
    renderUnderAToastRegion();

    await screen.findByText(/Discovery call with Turbinenbau/);
    expect(
      await screen.findByRole("button", { name: /update/i }),
    ).not.toBeNull();
    expect(
      screen.getByRole("button", {
        name: "Mark as canceled: Discovery call with Turbinenbau",
      }),
    ).not.toBeNull();
    // The two statuses that need prose are answers INSIDE the composer, not
    // verbs on the card: a row that still offered them would write one word and
    // close, which is the whole behaviour this replaced.
    expect(screen.queryByRole("button", { name: /it happened/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /didn't come/i })).toBeNull();
  });

  it("records a cancellation from the card, with no dialog in the way", async () => {
    const sent: unknown[] = [];
    stubWithMeetingRead(sent);
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", {
        name: "Mark as canceled: Discovery call with Turbinenbau",
      }),
    );
    expect(await screen.findByText(/Meeting outcome recorded/i)).not.toBeNull();
    expect(sent).toEqual([{ meeting_status: "canceled" }]);
  });

  it("opens the composer on THIS meeting, seeded from what was captured", async () => {
    stubWithMeetingRead([]);
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(await screen.findByRole("button", { name: /update/i }));
    // Seeded rather than blank: the reader is amending what the calendar
    // already knew, and an empty form invites them to save nothing over it.
    const subject = await screen.findByDisplayValue(
      "Discovery call with Turbinenbau",
    );
    expect(subject).not.toBeNull();
    await waitFor(() =>
      expect(messageText("Details")).toMatch(
        /Organizer: greta@turbinenbau\.example/,
      ),
    );
  });

  it("patches the meeting with the outcome rather than logging a second one", async () => {
    const sent: unknown[] = [];
    stubWithMeetingRead(sent);
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(await screen.findByRole("button", { name: /update/i }));
    await screen.findByDisplayValue("Discovery call with Turbinenbau");
    writeMessage("Details", "They want a pilot in Q1.");
    await user.click(screen.getByRole("button", { name: /^log$/i }));

    expect(await screen.findByText(/Meeting outcome recorded/i)).not.toBeNull();
    // ONE write, and it carries the outcome. A POST would have created a second
    // activity and left this meeting in the queue.
    expect(sent).toHaveLength(1);
    expect(sent[0]).toMatchObject({
      meeting_status: "held",
      subject: "Discovery call with Turbinenbau",
      body: "They want a pilot in Q1.",
    });
  });

  // A transcript's lines are what a citation points at, so the save sends it
  // byte for byte, leading blank line and indentation included.
  it("sends a transcript's text untrimmed", async () => {
    const sent: unknown[] = [];
    const transcript = "\n  Anna: hello\nBen: hi";
    stubWithMeetingRead(sent, 200, {
      ...CAPTURED_MEETING,
      source_system: "transcript",
      body: transcript,
    });
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(await screen.findByRole("button", { name: /update/i }));
    expect(
      (await screen.findByLabelText<HTMLTextAreaElement>("Details")).value,
    ).toBe(transcript);
    await user.click(screen.getByRole("button", { name: /^log$/i }));

    expect(await screen.findByText(/Meeting outcome recorded/i)).not.toBeNull();
    expect(sent[0]).toMatchObject({ body: transcript });
  });

  it("saves a body nobody edited exactly as it was read", async () => {
    const sent: unknown[] = [];
    const body = "Notes on snake_case naming\n\n5. Fifth\n6. Sixth";
    stubWithMeetingRead(sent, 200, { ...CAPTURED_MEETING, body });
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(await screen.findByRole("button", { name: /update/i }));
    await waitFor(() => expect(messageText("Details")).toContain("Fifth"));
    fireEvent.focus(messageBox("Details"));
    fireEvent.blur(messageBox("Details"));
    await user.click(screen.getByRole("button", { name: /^log$/i }));

    expect(await screen.findByText(/Meeting outcome recorded/i)).not.toBeNull();
    expect(sent[0]).toMatchObject({ body });
  });

  it("says so when the write is refused, rather than falling silent", async () => {
    stubWithMeetingRead([], 403);
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", {
        name: "Mark as canceled: Discovery call with Turbinenbau",
      }),
    );
    // A refused write leaves the row exactly as it was, which renders the same
    // as a click that did nothing.
    expect(await screen.findByText(/Outcome was not recorded/i)).not.toBeNull();
  });

  it("reads the queue again when the meeting changed under the row, so a retry carries its new version", async () => {
    // The server holds version 5; the row still carries 3.
    stubWithMeetingRead([], 200, { ...CAPTURED_MEETING, version: 5 });
    const worklistReads = () =>
      vi
        .mocked(globalThis.fetch)
        .mock.calls.filter(([input]) =>
          String(input instanceof Request ? input.url : input).includes(
            "/worklist",
          ),
        ).length;
    const user = userEvent.setup();
    renderUnderAToastRegion();
    const cancel = await screen.findByRole("button", {
      name: "Mark as canceled: Discovery call with Turbinenbau",
    });
    const before = worklistReads();

    await user.click(cancel);

    expect(await screen.findByText(/Outcome was not recorded/i)).not.toBeNull();
    await waitFor(() => expect(worklistReads()).toBeGreaterThan(before));
  });

  it("stops offering a cancel the calendar invitation refuses, and offers no retry", async () => {
    const sent: unknown[] = [];
    stubWithMeetingRead(sent, 200, {
      ...CAPTURED_MEETING,
      invitation_status: "confirmed",
    });
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", {
        name: "Mark as canceled: Discovery call with Turbinenbau",
      }),
    );
    expect(
      await screen.findByText(/Cancel it through the invitation/),
    ).not.toBeNull();
    expect(screen.queryByText(/Retry/)).toBeNull();
    expect(screen.queryByRole("button", { name: /canceled/ })).toBeNull();
    expect(screen.getByRole("button", { name: /update/i })).not.toBeNull();
  });
});

// The confirmation and the refusal both live in a toast, and renderWorklist
// mounts no region — the testkit is production-shaped for the conformance gate,
// which allows one ToastProvider and one ToastRegion and names main.tsx as
// their home. Mounted here, in a file the gate does not scan.
function renderUnderAToastRegion(
  zone = "Asia/Bangkok",
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  }),
) {
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordZoneProvider zone={zone}>
          <ToastProvider>
            <WorklistScreen />
            <ToastRegion />
          </ToastProvider>
        </RecordZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// Marking a meeting held must not move its start. The dialog sends a start
// only when the reader moved the day. It then keeps the meeting's time of day
// in the record zone.
describe("the start a meeting outcome saves", () => {
  type User = ReturnType<typeof userEvent.setup>;

  async function openAndSave(
    zone: string,
    moveDay?: (user: User) => Promise<void>,
  ): Promise<void> {
    const user = userEvent.setup();
    renderUnderAToastRegion(zone);
    await user.click(await screen.findByRole("button", { name: /update/i }));
    await screen.findByDisplayValue("Discovery call with Turbinenbau");
    await moveDay?.(user);
    await user.click(screen.getByRole("button", { name: /^log$/i }));
  }

  // The day as an English reader types it, over the one the box shows.
  const typed = (day: string) => async (user: User) => {
    await user.tripleClick(screen.getByRole("textbox", { name: "Date" }));
    await user.keyboard(day.split("-").reverse().join("/"));
  };

  const picked = (dayName: string) => async (user: User) => {
    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    await user.click(await screen.findByRole("button", { name: dayName }));
  };

  it.each(["Asia/Bangkok", "America/New_York"])(
    "sends no start when the day stayed, in %s",
    async (zone) => {
      const sent: Record<string, unknown>[] = [];
      stubWithMeetingRead(sent);
      await openAndSave(zone);

      expect(
        await screen.findByText(/Meeting outcome recorded/i),
      ).not.toBeNull();
      expect(sent).toHaveLength(1);
      expect(sent[0]).not.toHaveProperty("occurred_at");
    },
  );

  it.each(["Asia/Bangkok", "America/New_York"])(
    "keeps the time of day on the next day, in %s",
    async (zone) => {
      const sent: Record<string, unknown>[] = [];
      stubWithMeetingRead(sent);
      await openAndSave(zone, picked("Friday, 16 January 2026"));

      expect(
        await screen.findByText(/Meeting outcome recorded/i),
      ).not.toBeNull();
      expect(sent[0]).toMatchObject({
        occurred_at: "2026-01-16T14:00:00.000Z",
        occurred_precision: "instant",
      });
    },
  );

  it("moves a date-only meeting to its new date alone and keeps it date-only", async () => {
    const sent: Record<string, unknown>[] = [];
    // Noon on the 15th in Bangkok, as the server stores a date-only meeting.
    stubWithMeetingRead(sent, 200, {
      ...CAPTURED_MEETING,
      occurred_at: "2026-01-15T05:00:00Z",
      occurred_precision: "day",
    });
    await openAndSave("Asia/Bangkok", picked("Friday, 16 January 2026"));

    expect(await screen.findByText(/Meeting outcome recorded/i)).not.toBeNull();
    expect(sent[0]).toMatchObject({
      occurred_at: "2026-01-16T12:00:00Z",
      occurred_precision: "day",
    });
  });

  // A date-only meeting has taken place once its date is over, so the server
  // refuses `held` on one dated today.
  const dateOnly = {
    ...CAPTURED_MEETING,
    occurred_at: "2026-01-15T05:00:00Z",
    occurred_precision: "day",
  };

  it("refuses to move a date-only meeting onto today, and sends nothing", async () => {
    // 05:00 on the 20th in Bangkok, still the 19th in UTC.
    vi.useFakeTimers({ toFake: ["Date"], shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-01-19T22:00:00Z"));
    const sent: Record<string, unknown>[] = [];
    stubWithMeetingRead(sent, 200, dateOnly);
    await openAndSave("Asia/Bangkok", typed("2026-01-20"));

    expect(await screen.findByText("Latest date: 19/01/2026")).not.toBeNull();
    expect(sent).toEqual([]);
  });

  it("files a date-only meeting moved onto yesterday as that date alone", async () => {
    vi.useFakeTimers({ toFake: ["Date"], shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-01-19T22:00:00Z"));
    const sent: Record<string, unknown>[] = [];
    stubWithMeetingRead(sent, 200, dateOnly);
    await openAndSave("Asia/Bangkok", typed("2026-01-19"));

    expect(await screen.findByText(/Meeting outcome recorded/i)).not.toBeNull();
    expect(sent[0]).toMatchObject({
      occurred_at: "2026-01-19T12:00:00Z",
      occurred_precision: "day",
    });
  });

  it("files a meeting moved onto today a minute before now while its hour is still to come", async () => {
    // 17:00 in Bangkok; the meeting's 21:00 has not come yet.
    vi.useFakeTimers({ toFake: ["Date"], shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-01-20T10:00:00Z"));
    const sent: Record<string, unknown>[] = [];
    stubWithMeetingRead(sent);
    await openAndSave("Asia/Bangkok", typed("2026-01-20"));

    expect(await screen.findByText(/Meeting outcome recorded/i)).not.toBeNull();
    const filed = Date.parse(String(sent[0]?.occurred_at));
    expect(filed).toBeGreaterThanOrEqual(Date.parse("2026-01-20T09:59:00Z"));
    expect(filed).toBeLessThan(Date.parse("2026-01-20T10:00:00Z"));
  });

  it("files a meeting moved onto today at its own time once that hour passed", async () => {
    // 22:00 in Bangkok; the meeting's 21:00 is an hour ago.
    vi.useFakeTimers({ toFake: ["Date"], shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-01-20T15:00:00Z"));
    const sent: Record<string, unknown>[] = [];
    stubWithMeetingRead(sent);
    await openAndSave("Asia/Bangkok", typed("2026-01-20"));

    expect(await screen.findByText(/Meeting outcome recorded/i)).not.toBeNull();
    expect(sent[0]).toMatchObject({
      occurred_at: "2026-01-20T14:00:00.000Z",
    });
  });

  it("marks the record's today, under the cap, while the browser is a day ahead", async () => {
    // 10:00Z is the 19th at 23:00 in Pago Pago and the 20th from UTC-10 eastward.
    vi.useFakeTimers({ toFake: ["Date"], shouldAdvanceTime: true });
    vi.setSystemTime(new Date("2026-01-20T10:00:00Z"));
    stubWithMeetingRead([]);
    const user = userEvent.setup();
    renderUnderAToastRegion("Pacific/Pago_Pago");
    await user.click(await screen.findByRole("button", { name: /update/i }));
    await user.click(
      await screen.findByRole("button", { name: /^Choose date/ }),
    );
    const today = await screen.findByRole("button", { name: "Today" });
    expect(today).toHaveProperty("disabled", false);
    await user.click(today);

    expect(
      screen.getByRole<HTMLInputElement>("textbox", { name: "Date" }).value,
    ).toBe("19/01/2026");
  });

  it("sends nothing when the picked day does not exist in the record zone", async () => {
    // Apia skipped 30 December 2011 when it moved across the date line.
    const sent: Record<string, unknown>[] = [];
    stubWithMeetingRead(sent, 200, {
      ...CAPTURED_MEETING,
      occurred_at: "2011-12-29T22:00:00Z",
    });
    await openAndSave("Pacific/Apia", typed("2011-12-30"));

    expect(
      await screen.findByText(/does not exist in the company’s time zone/i),
    ).not.toBeNull();
    expect(sent).toEqual([]);
  });

  it("checks the save against the version the form was seeded from", async () => {
    const sent: Record<string, unknown>[] = [];
    const preconditions: (string | null)[] = [];
    // The server already holds version 5: a calendar replay moved the
    // meeting while the dialog was open.
    stubWithMeetingRead(
      sent,
      200,
      { ...CAPTURED_MEETING, version: 5 },
      preconditions,
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    client.setQueryData(["activity", MEETING_ID], CAPTURED_MEETING);
    const user = userEvent.setup();
    renderUnderAToastRegion("Asia/Bangkok", client);
    await user.click(await screen.findByRole("button", { name: /update/i }));
    await screen.findByDisplayValue("Discovery call with Turbinenbau");
    client.setQueryData(["activity", MEETING_ID], {
      ...CAPTURED_MEETING,
      version: 5,
      occurred_at: "2026-01-15T15:00:00Z",
    });
    await user.click(screen.getByRole("button", { name: /^log$/i }));

    expect(await screen.findByRole("alert")).not.toBeNull();
    expect(preconditions).toEqual(["3"]);
    expect(screen.queryByText(/Meeting outcome recorded/i)).toBeNull();
  });
});
