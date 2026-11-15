// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { dueInstant } from "../format/calendarday";
import { browserZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { MeetingOutcome } from "./meetingoutcome";
import { writeMeetingOutcome } from "./taskactions.rowwrites";

// The real writer, watched: the dialog must write through it, so a meeting's
// outcome has one writer.
vi.mock("./taskactions.rowwrites", async (original) => {
  const real = await original<typeof import("./taskactions.rowwrites")>();
  return { ...real, writeMeetingOutcome: vi.fn(real.writeMeetingOutcome) };
});

const MEETING_ID = "01a05500-0000-7000-8000-0000000000c1";

type Sent = {
  method: string;
  url: string;
  ifMatch: string | null;
  body: unknown;
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  vi.useRealTimers();
});

// Records every request the card makes, and answers a PATCH as the server
// does: with the meeting at its next version.
function stubServer(sent: Sent[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      const text = await request.clone().text();
      sent.push({
        method: request.method,
        url: new URL(request.url).pathname,
        ifMatch: request.headers.get("If-Match"),
        body: text === "" ? undefined : JSON.parse(text),
      });
      if (request.method === "GET") {
        return new Response(
          JSON.stringify({
            id: MEETING_ID,
            kind: "meeting",
            subject: "Site visit",
            body: "Agenda",
            occurred_at: "2026-07-06T09:00:00Z",
            occurred_precision: "instant",
            meeting_status: "booked",
            version: 7,
            writable: true,
            offers_email: false,
            source: "manual",
            captured_by: "human:u1",
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      return new Response(
        JSON.stringify({ id: MEETING_ID, meeting_status: "held", version: 8 }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    }),
  );
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <MeetingOutcome
            id={MEETING_ID}
            version={7}
            title="Site visit"
            invalidateKeys={[]}
            cancellable
          />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("MeetingOutcome", () => {
  // The choice records what happened; "Details" below it is the notes.
  it("names the Held or No-show choice for what it records", async () => {
    const user = userEvent.setup();
    stubServer([]);
    renderCard();

    await user.click(screen.getByRole("button", { name: /Add notes/ }));
    const dialog = await screen.findByRole("dialog");

    expect(
      await within(dialog).findByRole("combobox", { name: "What happened" }),
    ).toBeTruthy();
  });

  it("records held without a task when the next step is skipped", async () => {
    const user = userEvent.setup();
    const sent: Sent[] = [];
    stubServer(sent);
    renderCard();

    await user.click(
      screen.getByRole("button", { name: "It happened: Site visit" }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("What is the next step?")).toBeTruthy();
    await user.click(
      within(dialog).getByRole("button", { name: "Skip next step" }),
    );
    expect(await screen.findByText("Meeting outcome recorded")).toBeTruthy();
    expect(sent.filter((request) => request.method === "PATCH")).toEqual([
      {
        method: "PATCH",
        url: `/v1/activities/${MEETING_ID}`,
        ifMatch: "7",
        body: { meeting_status: "held" },
      },
    ]);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("asks How did it go?, then draws It happened, Add notes and Did not happen", () => {
    stubServer([]);
    renderCard();

    expect(screen.getByText("How did it go?")).toBeTruthy();
    expect(
      screen.getAllByRole("button").map((button) => button.textContent),
    ).toEqual(["It happened", "Add notes", "Did not happen"]);
  });

  it("saves the notes dialog through the one outcome writer", async () => {
    const user = userEvent.setup();
    const sent: Sent[] = [];
    stubServer(sent);
    renderCard();

    await user.click(screen.getByRole("button", { name: /Add notes/ }));
    const dialog = await screen.findByRole("dialog");
    await screen.findByDisplayValue("Site visit");
    await user.click(within(dialog).getByRole("button", { name: "Log" }));

    await waitFor(() => expect(writeMeetingOutcome).toHaveBeenCalledTimes(1));
    expect(vi.mocked(writeMeetingOutcome).mock.calls[0]?.[0]).toMatchObject({
      id: MEETING_ID,
      version: 7,
      status: "held",
      subject: "Site visit",
      body: "Agenda",
    });
    const patches = sent.filter((request) => request.method === "PATCH");
    expect(patches).toHaveLength(1);
    expect(patches[0]?.body).toEqual({
      meeting_status: "held",
      subject: "Site visit",
      body: "Agenda",
    });
  });
  it("submits a typed next step through the same held outcome write", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-07-06T09:00:00Z"));
    const user = userEvent.setup();
    const sent: Sent[] = [];
    stubServer(sent);
    renderCard();
    await user.click(
      screen.getByRole("button", { name: "It happened: Site visit" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("checkbox", { name: "Add a follow-up task" }),
    );
    expect(
      within(dialog)
        .getByRole("button", { name: "Log" })
        .hasAttribute("disabled"),
    ).toBe(true);
    fireEvent.change(
      within(dialog).getByRole("textbox", { name: "Next step" }),
      { target: { value: "Send revised quote" } },
    );
    const dueBox = within(dialog).getByRole("textbox", { name: "Due date" });
    const dueField = dueBox.parentElement;
    if (!dueField) throw new Error("expected due date inside its field");
    await user.click(
      within(dueField).getByRole("button", { name: /^Choose date/ }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Friday, 10 July 2026" }),
    );
    await user.click(within(dialog).getByRole("button", { name: "Log" }));
    await waitFor(() =>
      expect(sent.filter((request) => request.method === "PATCH")).toHaveLength(
        1,
      ),
    );
    const writes = sent.filter((request) => request.method !== "GET");
    expect(writes).toEqual([
      {
        method: "PATCH",
        url: `/v1/activities/${MEETING_ID}`,
        ifMatch: "7",
        body: {
          meeting_status: "held",
          follow_up_task: {
            subject: "Send revised quote",
            body: null,
            due_at: dueInstant("2026-07-10", browserZone()),
            due_has_time: false,
            due_intent: {
              wall_clock: "2026-07-10T23:59:59",
              timezone: browserZone(),
            },
          },
        },
      },
    ]);
  });
});
