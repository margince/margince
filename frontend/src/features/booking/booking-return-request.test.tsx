/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { bookingProfile, bookingSlots } from "../../testing/book.testkit";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { BookingScreen } from "./book";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(page?: string) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <BookingScreen hostSlug="ada-lovelace" page={page} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function ask(name: string, email: string) {
  fireEvent.change(screen.getByLabelText(en["book.name"]), {
    target: { value: name },
  });
  fireEvent.change(screen.getByLabelText(en["book.email"]), {
    target: { value: email },
  });
  fireEvent.click(
    screen.getByRole("button", { name: en["book.returnRequestAction"] }),
  );
}

it("sends the name and address to the host's return path and says only to check the inbox", async () => {
  const sent: unknown[] = [];
  installFetchStub({
    "POST /public/booking/ada-lovelace/return": (body) => {
      sent.push(body);
      return new Response(null, { status: 202 });
    },
  });
  mount("return");

  expect(screen.getByText(en["book.returnRequestBody"])).toBeTruthy();
  ask("Wieder Da", "wieder@example.com");

  expect(await screen.findByText(en["book.returnRequestSent"])).toBeTruthy();
  expect(sent).toEqual([{ name: "Wieder Da", email: "wieder@example.com" }]);
  expect(screen.queryByLabelText(en["book.email"])).toBeNull();
});

it("answers a spent daily budget exactly as it answers any address", async () => {
  installFetchStub({
    "POST /public/booking/ada-lovelace/return": () =>
      jsonResponse({ status: 429, title: "Too many requests" }, 429),
  });
  mount("return");

  ask("Wieder Da", "wieder@example.com");

  expect(await screen.findByText(en["book.returnRequestSent"])).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.queryByLabelText(en["book.email"])).toBeNull();
});

it("shows any other refusal as an error, not as sent", async () => {
  installFetchStub({
    "POST /public/booking/ada-lovelace/return": () =>
      jsonResponse(
        { status: 422, title: "Validation failed", detail: "bad email" },
        422,
      ),
  });
  mount("return");

  ask("Wieder Da", "wieder@example.com");

  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(screen.queryByText(en["book.returnRequestSent"])).toBeNull();
});

it("leaves the booking page itself without a way to the return page", async () => {
  installFetchStub({
    "GET /public/booking/ada-lovelace/profile": () =>
      jsonResponse(bookingProfile),
    "GET /public/booking/ada-lovelace/availability": () =>
      jsonResponse({ slots: bookingSlots, truncated: false }),
  });
  mount();

  await waitFor(() =>
    expect(document.title).toBe("Book a meeting with Ada Lovelace"),
  );
  const hrefs = [...document.querySelectorAll("a")].map((a) =>
    a.getAttribute("href"),
  );
  expect(hrefs.filter((href) => href?.includes("return"))).toEqual([]);
  expect(screen.queryByText(en["book.returnRequestAction"])).toBeNull();
});
