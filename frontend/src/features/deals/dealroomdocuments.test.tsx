/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { DealRoomConversation } from "./dealroomconversation";
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
    await screen.findByText(englishCopy["room.docs.noFiles"]),
  ).toBeInTheDocument();
  expect(
    screen.getByText(englishCopy["room.docs.fileHint"]),
  ).toBeInTheDocument();

  await user.click(
    screen.getByRole("button", { name: englishCopy["room.docs.upload"] }),
  );
  expect(await screen.findByRole("dialog")).toBeInTheDocument();

  // And it lets go again. The upload is a detour on the way to the picker, so
  // a reader who thinks better of it lands back on the form they came from
  // rather than on a dialog with no way out.
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(
    screen.getByRole("button", { name: englishCopy["room.docs.upload"] }),
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
  expect(
    screen.queryByRole("button", { name: englishCopy["room.docs.upload"] }),
  ).toBeNull();
});

function roomEntry(id: string, title: string, reachesBuyer: boolean) {
  return {
    id,
    room_id: ROOM.id,
    attachment_id: `att-${id}`,
    group_key: "legal",
    title,
    position: 0,
    filename: `${id}.pdf`,
    reaches_buyer: reachesBuyer,
    source: "manual",
    version: 1,
    created_at: "2026-08-22T09:00:00Z",
    updated_at: "2026-08-22T09:00:00Z",
  };
}

// A file deleted on the deal stays in the room for the seller to remove. The
// buyer no longer gets it, so the tile says so and offers no question about it.
it("marks a room entry whose file the buyer no longer gets", async () => {
  vi.stubGlobal("fetch", (input: Request) => {
    const path = new URL(input.url).pathname;
    if (path.endsWith(`/deal-rooms/${ROOM.id}/documents`)) {
      return Promise.resolve(
        jsonResponse({
          data: [
            roomEntry("live", "Order form", true),
            roomEntry("gone", "Old MSA", false),
          ],
          page: {},
        }),
      );
    }
    return Promise.resolve(jsonResponse({ data: [], page: {} }));
  });
  render(<DealRoomConversation room={ROOM} refusal={undefined} />);

  const gone = await screen.findByRole("article", { name: "Old MSA" });
  const live = screen.getByRole("article", { name: "Order form" });
  expect(
    within(gone).getByText(englishCopy["room.docs.lapsed"]),
  ).toBeInTheDocument();
  expect(
    within(gone).queryByRole("button", {
      name: englishCopy["threads.askAbout"],
    }),
  ).toBeNull();
  expect(
    within(gone).getByRole("button", { name: "Remove Old MSA from the room" }),
  ).toBeInTheDocument();
  expect(within(live).queryByText(englishCopy["room.docs.lapsed"])).toBeNull();
  expect(
    within(live).getByRole("button", { name: englishCopy["threads.askAbout"] }),
  ).toBeInTheDocument();
});

function stubFilesAndRefusal() {
  const file = (id: string, title: string) => ({
    attachment: { id, title, filename: `${id}.pdf` },
  });
  const posts: string[] = [];
  vi.stubGlobal("fetch", (input: Request) => {
    const path = new URL(input.url).pathname;
    if (input.method === "POST") posts.push(path);
    if (
      input.method === "POST" &&
      path.endsWith("/deal-rooms/room-1/documents")
    ) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            status: 409,
            code: "deal_room_document_already_present",
            detail:
              "this file is already in the room; rename or regroup the existing entry instead",
          }),
          { status: 409, headers: { "Content-Type": "application/json" } },
        ),
      );
    }
    if (path.endsWith("/deals/deal-1/documents")) {
      return Promise.resolve(
        jsonResponse({
          data: [file("a-1", "Price sheet"), file("a-2", "Order form")],
          page: {},
        }),
      );
    }
    return Promise.resolve(jsonResponse({ data: [], page: {} }));
  });
  return posts;
}

it("says once, on the file, that the file is already in the room, until another is picked", async () => {
  const user = userEvent.setup();
  stubFilesAndRefusal();
  render(<AddDocument room={ROOM} refusal={undefined} />);
  const picker = await screen.findByRole("combobox", { name: /^File/ });
  await waitFor(() => expect(picker).toBeEnabled());
  await pickOption(user, picker, "Price sheet");
  await user.click(screen.getByRole("button", { name: "Add to room" }));

  expect(
    await screen.findAllByText(englishCopy["room.docs.alreadyPresent"]),
  ).toHaveLength(1);
  expect(picker).toHaveAttribute("aria-invalid", "true");
  expect(screen.queryByText(/rename or regroup/)).toBeNull();

  await pickOption(user, picker, "Order form");
  expect(
    screen.queryByText(englishCopy["room.docs.alreadyPresent"]),
  ).toBeNull();
});

it("says the file is required when added with none picked, and sends nothing", async () => {
  const user = userEvent.setup();
  const posts = stubFilesAndRefusal();
  render(<AddDocument room={ROOM} refusal={undefined} />);
  const picker = await screen.findByRole("combobox", { name: /^File/ });
  await waitFor(() => expect(picker).toBeEnabled());
  await user.click(screen.getByRole("button", { name: "Add to room" }));

  expect(screen.getByText("File from this deal is required.")).toBeTruthy();
  expect(document.activeElement).toBe(picker);
  expect(posts).toHaveLength(0);
});
