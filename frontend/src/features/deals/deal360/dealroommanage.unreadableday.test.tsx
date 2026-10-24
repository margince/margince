// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { ManageRoomDialog } from "./dealroommanage";

const ROOM: components["schemas"]["DealRoom"] = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Nordwind fleet telematics",
  state: "live",
  expires_at: "2026-12-31T23:59:59Z",
  source: "manual",
  captured_by: "u-me",
  version: 1,
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-24T14:02:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubApi() {
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
  return writes;
}

it("refuses a save while the end day cannot be read, and keeps the stored day", async () => {
  const writes = stubApi();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ManageRoomDialog room={ROOM} open onClose={vi.fn()} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  const box = await screen.findByRole("textbox", { name: "Access ends on" });
  expect(box).toHaveValue("31/12/2026");
  await user.clear(box);
  await user.type(box, "31/02/2027");

  await user.click(screen.getByRole("button", { name: "Save changes" }));

  expect(writes).toHaveLength(0);
  expect(screen.getByRole("alert")).toHaveTextContent(/^Enter a date as /);
  expect(box).toHaveAttribute("aria-invalid", "true");
  expect(box).toHaveValue("31/02/2027");
});
