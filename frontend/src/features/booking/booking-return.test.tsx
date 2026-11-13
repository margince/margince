/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { BookingScreen } from "./book";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  history.replaceState(null, "", "/");
});

const preview = {
  name: "Wieder Da",
  wording: "You agree that we store your name again.",
  policy_version: "erasure-return-2026-11",
};
const spent = { status: 422, code: "validation_error", detail: "spent" };

function renderReturnLink(token: string) {
  history.replaceState(null, "", `/#/book/dana?return=${token}`);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <BookingScreen hostSlug="dana" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("shows the name and the signed wording, and stores only on the button", async () => {
  const confirmed: unknown[] = [];
  installFetchStub({
    "POST /public/booking/dana/return/preview": () => jsonResponse(preview),
    "POST /public/booking/dana/return/confirm": (body) => {
      confirmed.push(body);
      return new Response(null, { status: 204 });
    },
  });
  renderReturnLink("signed.token");

  expect(await screen.findByText(preview.wording)).toBeTruthy();
  expect(screen.getByText(/Wieder Da/)).toBeTruthy();
  expect(location.hash).toBe("#/book/dana");
  expect(confirmed).toEqual([]);
  fireEvent.click(
    screen.getByRole("button", { name: en["book.returnAction"] }),
  );

  expect(await screen.findByText(en["book.returnDone"])).toBeTruthy();
  expect(confirmed).toEqual([{ token: "signed.token" }]);
});

it("offers no button for a link that no longer works", async () => {
  installFetchStub({
    "POST /public/booking/dana/return/preview": () => jsonResponse(spent, 422),
  });
  renderReturnLink("spent.token");

  expect(await screen.findByText(en["book.returnSpent"])).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: en["book.returnAction"] }),
  ).toBeNull();
});
