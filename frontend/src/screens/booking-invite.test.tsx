/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { bookingContact, bookingProfile, bookingSlots } from "./book.testkit";
import { BookingInviteScreen } from "./booking-invite";

function mount(configured = true) {
  const proposals: unknown[] = [];
  const paths: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      paths.push(path);
      let body: unknown;
      if (request.method === "POST") {
        proposals.push(await request.json());
        body = {
          id: "proposal-1",
          url: "https://crm.example.test/#/book/proposal-private",
          expires_at: "2026-10-06T09:00:00Z",
        };
      } else if (path.includes("/contacts/"))
        body = {
          ...bookingContact,
          primary_email: null,
          emails: [{ email: "nina@brandt.example", is_primary: false }],
        };
      else if (path.endsWith("/availability"))
        body = { slots: bookingSlots, truncated: false };
      else body = { ...bookingProfile, provider: configured ? "gcal" : "" };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <BookingInviteScreen contactId={bookingContact.id} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { proposals, paths };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("prefills a singleton contact email and saved meeting details, then binds the selected proposal", async () => {
  const user = userEvent.setup();
  const { proposals } = mount();
  expect(await screen.findByDisplayValue("nina@brandt.example")).toBeTruthy();
  expect(await screen.findByDisplayValue(bookingProfile.title)).toBeTruthy();
  expect(screen.getByDisplayValue(bookingProfile.location)).toBeTruthy();
  expect(screen.getByText(/0 selected/)).toBeTruthy();
  for (const slot of bookingSlots)
    await user.click(
      await screen.findByRole("button", {
        name: formatDateTime(slot.start, "en", viewerZone()).replace(
          /\s+/g,
          " ",
        ),
      }),
    );
  expect(screen.getByText(/2 selected/)).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Create proposal" }));
  await waitFor(() => expect(proposals).toHaveLength(1));
  expect(proposals[0]).toMatchObject({
    contact_id: bookingContact.id,
    attendee_email: "nina@brandt.example",
    subject: bookingProfile.title,
    location: bookingProfile.location,
    options: bookingSlots,
  });
  expect(
    await screen.findByRole("link", { name: "Open personal invitation" }),
  ).toBeTruthy();
  await user.type(screen.getByLabelText("Meeting title"), " updated");
  expect(
    screen.getByRole("link", { name: "Open personal invitation" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Create updated proposal" }),
  ).toBeTruthy();
  expect(proposals).toHaveLength(1);
});

it("explains calendar setup without requesting unavailable times", async () => {
  const { paths } = mount(false);
  expect(
    await screen.findByRole("button", {
      name: "Connect or reconnect calendar",
    }),
  ).toBeTruthy();
  expect(screen.getByText(/Calendar write access is required/)).toBeTruthy();
  expect(paths.some((path) => path.endsWith("/availability"))).toBe(false);
});
