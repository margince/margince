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
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";
import { day, row, stub } from "./worklist.testkit";

const MEETING_ID = "01a05500-0000-7000-8000-0000000000b1";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
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
  occurred_at: "2026-08-31T08:00:00Z",
  meeting_status: "booked",
  version: 3,
  source: "gcal",
  captured_by: "connector:gcal",
  created_at: "2026-08-31T08:00:00Z",
  updated_at: "2026-08-31T08:00:00Z",
};

// Serves the worklist read, the meeting read behind the dialog, and records
// every PATCH body so a test can assert what the server was actually told.
function stubWithMeetingRead(sent: unknown[], patchStatus = 200) {
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
          if (patchStatus !== 200) {
            return new Response(JSON.stringify({ title: "no" }), {
              status: patchStatus,
              headers: { "content-type": "application/problem+json" },
            });
          }
          return new Response(
            JSON.stringify({ ...CAPTURED_MEETING, version: 4 }),
            { status: 200, headers: { "content-type": "application/json" } },
          );
        }
        return new Response(JSON.stringify(CAPTURED_MEETING), {
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
    expect(screen.getByRole("button", { name: /^canceled$/i })).not.toBeNull();
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
      await screen.findByRole("button", { name: /^canceled$/i }),
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
    expect(
      screen.getByDisplayValue(/Organizer: greta@turbinenbau\.example/),
    ).not.toBeNull();
  });

  it("patches the meeting with the outcome rather than logging a second one", async () => {
    const sent: unknown[] = [];
    stubWithMeetingRead(sent);
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(await screen.findByRole("button", { name: /update/i }));
    const details = await screen.findByDisplayValue(
      /Organizer: greta@turbinenbau\.example/,
    );
    await user.clear(details);
    await user.type(details, "They want a pilot in Q1.");
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

  it("says so when the write is refused, rather than falling silent", async () => {
    stubWithMeetingRead([], 403);
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", { name: /^canceled$/i }),
    );
    // A refused write leaves the row exactly as it was, which renders the same
    // as a click that did nothing.
    expect(await screen.findByText(/Outcome was not recorded/i)).not.toBeNull();
  });
});

// The confirmation and the refusal both live in a toast, and renderWorklist
// mounts no region — the testkit is production-shaped for the conformance gate,
// which allows one ToastProvider and one ToastRegion and names main.tsx as
// their home. Mounted here, in a file the gate does not scan.
function renderUnderAToastRegion() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
