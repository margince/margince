/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { createQueryClient } from "../app/queryclient";
import { BookingScreen } from "./book";
import {
  bookingConnection,
  bookingHours,
  bookingProfile,
} from "./book.testkit";
import { MeetingSettings } from "./meeting-settings";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it.each([false, true])(
  "previews the saved page with enabled=%s without public reads or writes",
  async (enabled) => {
    const user = userEvent.setup();
    const publicRead = vi.fn(() => jsonResponse(bookingProfile));
    const booking = vi.fn(() => jsonResponse({}));
    installFetchStub({
      "GET /scheduling/profile": () =>
        jsonResponse({ ...bookingProfile, enabled }),
      "GET /public/booking/ada-lovelace/profile": publicRead,
      "GET /public/booking/ada-lovelace/availability": publicRead,
      "POST /public/booking/ada-lovelace": booking,
    });
    render(
      <StoryProviders>
        <BookingScreen hostSlug="preview" />
      </StoryProviders>,
    );
    expect(
      await screen.findByRole("heading", { name: bookingProfile.title }),
    ).toBeTruthy();
    expect(
      screen.getByText(
        enabled ? /booking page is live/ : /Public booking is paused/,
      ),
    ).toBeTruthy();
    const button = screen.getByRole("button", { name: "Confirm meeting" });
    expect(button).toHaveProperty("disabled", true);
    await user.click(button);
    expect(publicRead).not.toHaveBeenCalled();
    expect(booking).not.toHaveBeenCalled();
  },
);
it("keeps a paused public page unavailable to visitors", async () => {
  installFetchStub({
    "GET /public/booking/ada-lovelace/profile": () =>
      jsonResponse({ ...bookingProfile, enabled: false }),
  });
  render(
    <StoryProviders>
      <BookingScreen hostSlug="ada-lovelace" />
    </StoryProviders>,
  );
  expect(await screen.findByText(/unavailable/i)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Confirm meeting" })).toBeNull();
});

it("returns from settings to a preview with the newly saved values in a fresh cache", async () => {
  const user = userEvent.setup();
  let latest: unknown = { ...bookingProfile, enabled: false };
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }),
    "GET /me/working-hours": () => jsonResponse(bookingHours),
    "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
    "GET /scheduling/calendars": () =>
      jsonResponse([
        { id: "primary", name: "Work", primary: true, writable: true },
      ]),
    "GET /scheduling/profile": () => jsonResponse(latest),
    "PUT /scheduling/profile": (body) => {
      latest = body;
      return jsonResponse(body);
    },
  });
  const client = createQueryClient();
  client.setDefaultOptions({ queries: { staleTime: Infinity, retry: false } });
  function Journey() {
    const [preview, setPreview] = useState(true);
    return (
      <>
        <button type="button" onClick={() => setPreview(!preview)}>
          Switch view
        </button>
        {preview ? <BookingScreen hostSlug="preview" /> : <MeetingSettings />}
      </>
    );
  }
  render(
    <StoryProviders>
      <QueryClientProvider client={client}>
        <Journey />
      </QueryClientProvider>
    </StoryProviders>,
  );
  await screen.findByRole("heading", { name: bookingProfile.title });
  await user.click(screen.getByRole("button", { name: "Switch view" }));
  const title = await screen.findByLabelText("Meeting title");
  await user.clear(title);
  await user.type(title, "Newly saved title");
  await user.click(screen.getByRole("button", { name: "Save settings" }));
  await waitFor(() =>
    expect(latest).toMatchObject({ title: "Newly saved title" }),
  );
  await user.click(screen.getByRole("button", { name: "Switch view" }));
  expect(
    await screen.findByRole("heading", { name: "Newly saved title" }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("heading", { name: bookingProfile.title }),
  ).toBeNull();
});
