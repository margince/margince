/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { AddDocument } from "./dealroomdocuments";

// A room shares the DEAL's files and holds none of its own, so a rep whose deal
// has no files yet has two steps to take and the form has to name both. It used
// to name only the second: the picker said the Files area was empty and offered
// nothing to do about it, and the upload that would fix it lived on another tab
// with nothing on this screen pointing at it.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type DealRoom = components["schemas"]["DealRoom"];

const ROOM = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Acme expansion",
  state: "live",
  source: "manual",
  version: 1,
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-22T09:00:00Z",
} as DealRoom;

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// The deal's Files area, empty — the state this affordance exists for — plus
// the session and installation reads the upload dialog makes when it opens.
function stubApi() {
  vi.stubGlobal("fetch", (input: Request) => {
    const path = new URL(input.url).pathname;
    if (path.endsWith("/me")) {
      return Promise.resolve(
        jsonResponse({
          user: { id: "u1" },
          authorization: {
            seat_type: "full",
            objects: { deal: { create: true, read: true, update: true } },
          },
        }),
      );
    }
    if (path.endsWith("/installation")) {
      return Promise.resolve(jsonResponse({ max_upload_bytes: 25_000_000 }));
    }
    return Promise.resolve(jsonResponse({ data: [], page: {} }));
  });
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

it("offers the upload that fills an empty Files area, and says where it lands", async () => {
  const user = userEvent.setup();
  stubApi();
  render(<AddDocument room={ROOM} refusal={undefined} />);

  expect(
    await screen.findByText("The deal's Files area is empty"),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      "Anything in the deal's Files area can go in: uploads and the files its emails carried. Upload a file to put a new one there.",
    ),
  ).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Upload a file" }));
  expect(await screen.findByRole("dialog")).toBeInTheDocument();

  // And it lets go again. The upload is a detour on the way to the picker, so
  // a reader who thinks better of it lands back on the form they came from
  // rather than on a dialog with no way out.
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(
    screen.getByRole("button", { name: "Upload a file" }),
  ).toBeInTheDocument();
});

it("a refused room offers no upload, only the sentence saying why", () => {
  stubApi();
  render(
    <AddDocument
      room={ROOM}
      refusal="This room is closed, so its documents can no longer change."
    />,
  );

  expect(
    screen.getByText(
      "This room is closed, so its documents can no longer change.",
    ),
  ).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Upload a file" })).toBeNull();
});
