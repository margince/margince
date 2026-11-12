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

it("stores the subject only when they press the button, and takes the token out of the address", async () => {
  const sent: unknown[] = [];
  installFetchStub({
    "POST /public/booking/dana/return/confirm": (body) => {
      sent.push(body);
      return new Response(null, { status: 204 });
    },
  });
  renderReturnLink("signed.token");

  expect(location.hash).toBe("#/book/dana");
  expect(sent).toEqual([]);
  fireEvent.click(
    screen.getByRole("button", { name: en["book.returnAction"] }),
  );

  expect(await screen.findByText(en["book.returnDone"])).toBeTruthy();
  expect(sent).toEqual([{ token: "signed.token" }]);
});

it("says a spent link is spent and offers no second try", async () => {
  installFetchStub({
    "POST /public/booking/dana/return/confirm": () =>
      jsonResponse(
        { status: 422, code: "validation_error", detail: "spent" },
        422,
      ),
  });
  renderReturnLink("spent.token");

  fireEvent.click(
    screen.getByRole("button", { name: en["book.returnAction"] }),
  );

  expect(await screen.findByText(en["book.returnSpent"])).toBeTruthy();
  expect(
    screen.getByRole("button", { name: en["book.returnAction"] }),
  ).toHaveProperty("disabled", true);
});
