/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { BookingScreen, PUBLIC_BOOKING_CONSENT } from "./book";
import {
  bookingInvitation,
  bookingProfile,
  bookingSlots,
} from "./book.testkit";

type Call = { method: string; path: string; body: unknown; key: string | null };
function serve(fail = false) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      const body =
        request.method === "POST" ? await request.clone().json() : null;
      calls.push({
        method: request.method,
        path,
        body,
        key: request.headers.get("Idempotency-Key"),
      });
      const post = request.method === "POST";
      const reply = post
        ? fail
          ? {
              status: 409,
              title: "Slot no longer available",
              detail: "Choose another time.",
            }
          : {
              invitation: {
                ...bookingInvitation,
                management_token: "private-link",
              },
            }
        : path.endsWith("/calendars")
          ? []
          : path.endsWith("/availability")
            ? { slots: bookingSlots, truncated: false }
            : path.includes("/meeting/")
              ? bookingInvitation
              : bookingProfile;
      return new Response(JSON.stringify(reply), {
        status: post ? (fail ? 409 : 201) : 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
  return calls;
}
function mount(hostSlug?: string) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <BookingScreen hostSlug={hostSlug} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
const slotName = (index: number) =>
  formatDateTime(bookingSlots[index].start, "en", viewerZone()).replace(
    /\s+/g,
    " ",
  );
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it("offers the reusable link and signature without requiring a contact", async () => {
  const calls = serve();
  mount();
  expect(
    await screen.findByDisplayValue(bookingProfile.public_url ?? ""),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Copy signature link" }),
  ).toBeTruthy();
  expect(calls.some((call) => call.path.includes("/contacts"))).toBe(false);
});
it("submits a real invitation only after the visitor confirms their details and consent", async () => {
  const user = userEvent.setup();
  const calls = serve();
  mount("ada-lovelace");
  await user.click(await screen.findByRole("button", { name: slotName(0) }));
  expect(calls.filter((call) => call.method === "POST")).toHaveLength(0);
  expect(
    screen.getByRole("button", { name: "Confirm meeting" }),
  ).toHaveProperty("disabled", true);
  await user.type(screen.getByLabelText("Your name"), "Nina Weber");
  await user.type(screen.getByLabelText("Your email"), "nina@brandt.example");
  await user.click(screen.getByRole("checkbox"));
  await user.click(screen.getByRole("button", { name: "Confirm meeting" }));
  await waitFor(() =>
    expect(window.location.hash).toContain("manage-private-link"),
  );
  const posted = calls.find((call) => call.method === "POST");
  expect(posted?.body).toMatchObject({
    ...bookingSlots[0],
    delivery: "calendar",
    booker: { name: "Nina Weber", email: "nina@brandt.example" },
    consent: PUBLIC_BOOKING_CONSENT,
  });
  expect(posted?.key).toBeTruthy();
});
it("retains the visitor's details and retry identity when a slot is refused", async () => {
  const user = userEvent.setup();
  const calls = serve(true);
  mount("ada-lovelace");
  await user.click(await screen.findByRole("button", { name: slotName(1) }));
  await user.type(screen.getByLabelText("Your name"), "Nina Weber");
  await user.type(screen.getByLabelText("Your email"), "nina@brandt.example");
  await user.click(screen.getByRole("checkbox"));
  await user.click(screen.getByRole("button", { name: "Confirm meeting" }));
  await screen.findByText("Choose another time.");
  expect(screen.getByLabelText("Your email")).toHaveProperty(
    "value",
    "nina@brandt.example",
  );
  await user.click(screen.getByRole("button", { name: "Confirm meeting" }));
  await waitFor(() =>
    expect(calls.filter((call) => call.method === "POST")).toHaveLength(2),
  );
  const posts = calls.filter((call) => call.method === "POST");
  expect(posts[0].key).toBe(posts[1].key);
  expect(window.location.hash).not.toContain("manage-");
});
it("keeps a pending provider operation distinct from a confirmed invitation", async () => {
  serve();
  mount("manage-private-link");
  expect(
    await screen.findByRole("heading", { name: "Creating your invitation…" }),
  ).toBeTruthy();
  expect(screen.queryByText("Calendar invitation created")).toBeNull();
  expect(
    screen
      .getByRole("button", { name: "Cancel meeting" })
      .hasAttribute("disabled"),
  ).toBe(false);
});

it("recovers the existing meeting from a used personal proposal", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            profile: { ...bookingProfile, enabled: false },
            used: true,
            options: [],
            description: "Project discovery",
            expires_at: "2026-10-06T09:00:00Z",
            meeting: {
              ...bookingInvitation,
              management_token: "recovered-link",
            },
          }),
          { headers: { "Content-Type": "application/json" } },
        ),
    ),
  );
  mount("proposal-personal-link");
  await user.click(
    await screen.findByRole("button", { name: "View your meeting" }),
  );
  expect(window.location.hash).toContain("manage-recovered-link");
});
