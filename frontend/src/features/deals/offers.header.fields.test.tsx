// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { offer } from "./deals.testkit";
import { EditOfferHeaderModal } from "./offers.header";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function openHeaderEdit(validUntil: string | null = null) {
  const writes: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.method !== "GET") {
        writes.push(request);
      }
      return Response.json({ data: [], page: { next_cursor: null } });
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <EditOfferHeaderModal
          open
          onClose={vi.fn()}
          offer={offer({ valid_until: validUntil, version: 3 })}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return writes;
}

// Send refuses an offer past its day, so the header refuses that day at entry.
it("refuses a valid-until day already past, and says the earliest day it takes", async () => {
  const writes = openHeaderEdit();
  const user = userEvent.setup();
  const box = await screen.findByRole("textbox", { name: "Valid until" });
  await user.type(box, "01/01/1999");

  await user.click(screen.getByRole("button", { name: "Save" }));

  expect(writes).toHaveLength(0);
  expect(screen.getByRole("alert")).toHaveTextContent(/^Earliest date: /);
  expect(box).toHaveAttribute("aria-invalid", "true");
});

// A day saved before it passed is the offer's own; a Save of other fields
// never writes it again.
it("leaves a saved valid-until day out of a Save that did not change it", async () => {
  const writes = openHeaderEdit("1999-01-01");
  const user = userEvent.setup();
  await user.type(
    await screen.findByRole("textbox", { name: "Terms text" }),
    "Net 30",
  );
  await user.click(screen.getByRole("button", { name: "Save" }));

  await vi.waitFor(() => expect(writes).toHaveLength(1));
  expect(await writes[0].json()).not.toHaveProperty("valid_until");
});

// The PDF prints these texts with their line breaks, so the field keeps them.
it("takes the intro and terms texts in fields that hold more than one line", async () => {
  openHeaderEdit();
  for (const name of ["Intro text", "Terms text"]) {
    expect((await screen.findByRole("textbox", { name })).tagName).toBe(
      "TEXTAREA",
    );
  }
});
