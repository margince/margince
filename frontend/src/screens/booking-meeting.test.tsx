// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { routeHash } from "../app/router";
import { formatDayFull, formatTimeRange } from "../format/meetingtime";
import { viewerZone } from "../format/timezone";
import { bookingInvitation } from "./book.testkit";
import { BookingMeetingScreen } from "./booking-meeting";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

type Invitation = components["schemas"]["MeetingInvitation"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const JOIN = "https://meet.google.com/abc-defg-hij";

function host(meeting: Partial<Invitation>) {
  const changes: unknown[] = [];
  installFetchStub({
    [`GET /scheduling/invitations/${bookingInvitation.id}`]: () =>
      jsonResponse({ ...bookingInvitation, ...meeting }),
    [`PATCH /scheduling/invitations/${bookingInvitation.id}`]: (body) => {
      changes.push(body);
      return jsonResponse({ ...bookingInvitation, ...meeting });
    },
  });
  render(
    <StoryProviders>
      <BookingMeetingScreen id={bookingInvitation.id} />
    </StoryProviders>,
  );
  return changes;
}

function guest(meeting: Partial<Invitation>) {
  installFetchStub({
    "GET /public/meeting/private-link": () =>
      jsonResponse({ ...bookingInvitation, ...meeting }),
  });
  render(
    <StoryProviders>
      <BookingMeetingScreen token="private-link" />
    </StoryProviders>,
  );
}

it("names the meeting in its heading, with its delivery as a badge and its time in full", async () => {
  host({ status: "confirmed" });
  expect(
    await screen.findByRole("heading", { level: 1, name: "Project discovery" }),
  ).toBeTruthy();
  expect(screen.getByRole("status").textContent).toBe(
    "Calendar invitation created",
  );
  const zone = viewerZone();
  expect(
    screen.getByText(
      `${formatDayFull(bookingInvitation.start, "en", zone)} · ${formatTimeRange(bookingInvitation.start, bookingInvitation.end, "en", zone)}`,
    ),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Back" })).toBeTruthy();
});

it("takes a meeting opened cold home rather than back out of the app", async () => {
  const user = userEvent.setup();
  const back = vi.spyOn(globalThis.history, "back");
  host({ status: "confirmed" });
  await user.click(await screen.findByRole("button", { name: "Back" }));
  expect(back).not.toHaveBeenCalled();
  expect(globalThis.location.hash).toBe(routeHash({ screen: "home" }));
});

it("shows the join link with a copy action once the calendar has made one", async () => {
  host({
    status: "confirmed",
    provider: "gcal",
    video_call: true,
    video_url: JOIN,
    calendar_url: "https://calendar.google.com/event?eid=1",
  });
  const link = await screen.findByRole("link", { name: JOIN });
  expect(link.getAttribute("href")).toBe(JOIN);
  expect(screen.getByText("Google Meet")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy();
  expect(
    screen.getByRole("link", { name: "Open in Google Calendar" }),
  ).toBeTruthy();
  // The agent's verb leads, and giving up on the meeting comes last.
  const verbs = screen
    .getAllByRole("button")
    .map((button) => button.textContent)
    .filter((name) =>
      ["Prepare for this meeting", "Reschedule", "Cancel meeting"].includes(
        name ?? "",
      ),
    );
  expect(verbs).toEqual([
    "Prepare for this meeting",
    "Reschedule",
    "Cancel meeting",
  ]);
});

it("says the link is coming while the calendar has not answered yet", async () => {
  host({ status: "pending", provider: "graphcal", video_call: true });
  expect(
    await screen.findByText(
      "The link appears once the calendar accepts the invitation.",
    ),
  ).toBeTruthy();
  expect(screen.getByText("Microsoft Teams")).toBeTruthy();
  const delivery = screen.getByRole("region", { name: "Delivery" });
  expect(within(delivery).getByText("Invitation created")).toBeTruthy();
  const current = delivery.querySelector('[aria-current="step"]');
  expect(current?.textContent).toBe("Sending to the calendar…");
});

it("tells the host when the calendar accepted without a link", async () => {
  host({ status: "confirmed", provider: "gcal", video_call: true });
  expect(
    await screen.findByText(
      "The calendar did not add a video link. Add one in your calendar.",
    ),
  ).toBeTruthy();
  // Accepted by the calendar is not accepted by the guest.
  const delivery = screen.getByRole("region", { name: "Delivery" });
  expect(delivery.querySelector('[aria-current="step"]')?.textContent).toBe(
    "Waiting for the guest’s reply",
  );
});

it("keeps the host's delivery and link notes off the guest's page", async () => {
  guest({ status: "confirmed", video_call: true });
  await screen.findByRole("heading", { level: 1, name: "Project discovery" });
  // The fixture's location is itself the words "Video call", so the row is
  // looked for by its term.
  expect(screen.queryByText("Video call", { selector: "dt" })).toBeNull();
  expect(screen.queryByRole("region", { name: "Delivery" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Back" })).toBeNull();
});

it("offers a retry of the same invitation before cancelling when the calendar refused it", async () => {
  const user = userEvent.setup();
  const changes = host({ status: "needs_attention", version: 4 });
  const notice = (
    await screen.findByText("The calendar did not accept the invitation")
  ).closest(".callout");
  if (!(notice instanceof HTMLElement))
    throw new Error("The refusal is explained in a callout");
  const retry = within(notice).getByRole("button", {
    name: "Retry invitation",
  });
  const cancel = screen.getByRole("button", { name: "Cancel meeting" });
  expect(
    retry.compareDocumentPosition(cancel) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  // A refused invitation never reached the guest, so nothing awaits a reply.
  const delivery = screen.getByRole("region", { name: "Delivery" });
  expect(
    within(delivery).queryByText("Waiting for the guest’s reply"),
  ).toBeNull();
  await user.click(retry);
  await waitFor(() =>
    expect(changes).toEqual([{ action: "retry", version: 4 }]),
  );
});
