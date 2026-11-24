/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { bookingProfile } from "../../testing/book.testkit";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { useBookingTitle } from "./booking-common";
import { BookingGuestScreen } from "./booking-guest";

// The app shell's own head, as index.html ships it, with its scripts left out.
const shellHead = new DOMParser()
  .parseFromString(
    readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), "../../../index.html"),
      "utf8",
    ),
    "text/html",
  )
  .head.innerHTML.replace(/<script[\s\S]*?<\/script>/g, "");

const LOGO = "https://crm.example.test/v1/public/booking/company-logo";
const acme = { paused: false, company_name: "Acme GmbH", logo_url: LOGO };

const content = (selector: string) =>
  document.head.querySelector<HTMLMetaElement>(selector)?.content;
const icons = () =>
  Array.from(
    document.head.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]'),
    (link) => link.getAttribute("href"),
  );

beforeEach(() => {
  document.head.innerHTML = shellHead;
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.history.replaceState(null, "", "/");
});

it("starts from a shell head that names Margince", () => {
  expect(document.title).toBe("Margince");
  expect(document.head.innerHTML).toContain("Margince");
});

it("names the booking and the host's company in the tab and the preview tags", () => {
  renderHook(() => useBookingTitle("Book a meeting with Ada", acme));
  expect(document.title).toBe("Book a meeting with Ada");
  expect(icons()).toEqual([LOGO]);
  expect(content('meta[property="og:title"]')).toBe("Book a meeting with Ada");
  expect(content('meta[property="og:site_name"]')).toBe("Acme GmbH");
  expect(content('meta[property="og:image"]')).toBe(LOGO);
  expect(content('meta[name="twitter:image"]')).toBe(LOGO);
  expect(document.head.innerHTML).not.toMatch(/margince|og\.png/i);
  expect(document.head.querySelector('link[rel="manifest"]')).toBeNull();
});

it.each([
  ["no company", undefined],
  ["a company with no logo", { ...acme, logo_url: null }],
  ["a paused page", { ...acme, paused: true }],
])("with %s, keeps a neutral head", (_case, brand) => {
  renderHook(() => useBookingTitle("Book a meeting", brand));
  expect(document.title).toBe("Book a meeting");
  expect(icons()).toEqual(["data:,"]);
  expect(content('meta[property="og:image"]')).toBeUndefined();
  if (brand?.paused !== false)
    expect(document.head.innerHTML).not.toContain("Acme");
  expect(document.head.innerHTML).not.toMatch(/margince|og\.png/i);
});

it("follows the brand once the profile arrives", () => {
  const { rerender } = renderHook(
    ({ brand }) => useBookingTitle("Book a meeting with Ada", brand),
    { initialProps: { brand: undefined as typeof acme | undefined } },
  );
  expect(icons()).toEqual(["data:,"]);
  rerender({ brand: acme });
  expect(icons()).toEqual([LOGO]);
  expect(
    document.head.querySelectorAll('meta[property="og:title"]'),
  ).toHaveLength(1);
});

it("gives the shell its own head back when the screen leaves", () => {
  const before = document.head.innerHTML;
  const { unmount } = renderHook(() => useBookingTitle("Your meeting", acme));
  unmount();
  expect(document.head.innerHTML).toBe(before);
  expect(document.title).toBe("Margince");
});

it("leaves the head alone for the host's own signed-in view", () => {
  const before = document.head.innerHTML;
  renderHook(() => useBookingTitle(null, acme));
  expect(document.head.innerHTML).toBe(before);
});

// The guest's meeting length on the public page.

const notOffered = {
  status: 422,
  code: "validation_error",
  detail: "This meeting length is no longer offered. Choose another length.",
  details: {
    errors: [{ field: "l", code: "length_not_offered", message: "gone" }],
  },
};

function guestProviders(client: QueryClient, children: ReactNode) {
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

// The server's rule: the asked length when the page offers it, else the
// default. A time read at a length it no longer offers is refused.
function mountGuest(address: string, offered = [30, 45, 60]) {
  globalThis.history.replaceState(null, "", address);
  const server = { offered, asked: [] as (string | null)[] };
  installFetchStub({
    "GET /public/booking/host/profile": (_body, url) => {
      const length = Number(url.searchParams.get("l"));
      return jsonResponse({
        ...bookingProfile,
        paused: false,
        offered_lengths: server.offered,
        duration_minutes: server.offered.includes(length) ? length : 30,
      });
    },
    "GET /public/booking/host/availability": (_body, url) => {
      const length = url.searchParams.get("l");
      server.asked.push(length);
      return server.offered.includes(Number(length))
        ? jsonResponse({ slots: [], truncated: false })
        : jsonResponse(notOffered, 422);
    },
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    guestProviders(client, <BookingGuestScreen hostSlug="host" />),
  );
  return { server, view, client };
}

const pressed = (name: string) =>
  screen.getByRole("button", { name }).getAttribute("aria-pressed");

it("asks for the times of the length the guest picks and keeps it in the address", async () => {
  const { server } = mountGuest("/book/host");
  await userEvent.click(await screen.findByRole("button", { name: "45 min" }));
  await waitFor(() => expect(server.asked).toContain("45"));
  expect(new URLSearchParams(globalThis.location.search).get("l")).toBe("45");
  await waitFor(() => expect(pressed("45 min")).toBe("true"));
  expect(pressed("30 min")).toBe("false");
});

it.each([
  ["opens at the length a link asks for", "/book/host?l=60", "60 min"],
  [
    "falls back to the default for a length not offered",
    "/book/host?l=47",
    "30 min",
  ],
])("%s", async (_name, address, chosen) => {
  const { server } = mountGuest(address);
  await waitFor(() => expect(pressed(chosen)).toBe("true"));
  for (const length of ["30 min", "45 min", "60 min"])
    if (length !== chosen) expect(pressed(length)).toBe("false");
  await waitFor(() => expect(server.asked).toContain(chosen.split(" ")[0]));
  expect(server.asked).not.toContain("47");
});

it("moves to a length still offered when the host removed the one the guest picked", async () => {
  const { server } = mountGuest("/book/host");
  await screen.findByRole("button", { name: "45 min" });
  server.offered = [30, 60];
  await userEvent.click(screen.getByRole("button", { name: "45 min" }));
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "45 min" })).toBeNull(),
  );
  await waitFor(() => expect(pressed("30 min")).toBe("true"));
  await waitFor(() => expect(server.asked.at(-1)).toBe("30"));
});

it("never shows the last host's page while the next host's page loads", async () => {
  const { view, client } = mountGuest("/book/host");
  expect(await screen.findByText(bookingProfile.title)).toBeTruthy();
  installFetchStub({
    "GET /public/booking/other/profile": () => new Promise(() => undefined),
  });
  view.rerender(
    guestProviders(client, <BookingGuestScreen hostSlug="other" />),
  );
  await waitFor(() =>
    expect(screen.queryByText(bookingProfile.title)).toBeNull(),
  );
  expect(screen.queryByRole("button", { name: "45 min" })).toBeNull();
});
