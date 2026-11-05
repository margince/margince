// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  focusManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider, translate } from "../../../i18n";
import { jsonResponse, stubWithSession } from "../../../testing/story-utils";
import { DealRoomTab } from "./dealroomtab";

// The deal record's own door into the Deal Room. It draws the room's band,
// what it still lacks, its brief, the documents, the conversation and who may
// enter. The room's settings open in a dialog.

type DealRoom = components["schemas"]["DealRoom"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function withProviders(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function room(over: Partial<DealRoom> = {}): DealRoom {
  return {
    id: "room-1",
    deal_id: "deal-1",
    title: "Acme Expansion — Deal Room",
    state: "live",
    source: "manual",
    version: 1,
    created_at: "2026-08-22T09:00:00Z",
    updated_at: "2026-08-22T09:00:00Z",
    ...over,
  } as DealRoom;
}

function roomRoutes(state: DealRoom["state"] = "live") {
  return {
    "GET /deal-rooms": () =>
      jsonResponse({ data: [room({ state })], page: { next_cursor: null } }),
    "GET /deal-rooms/room-1/participants": () =>
      jsonResponse({ data: [], page: {} }),
    "GET /deal-rooms/room-1/documents": () =>
      jsonResponse({ data: [], page: { next_cursor: null } }),
    "GET /deal-rooms/room-1/threads": () =>
      jsonResponse({ data: [], page: { next_cursor: null } }),
    "GET /deal-rooms/room-1/brief": () =>
      jsonResponse({
        room_id: "room-1",
        generated_at: "2026-10-22T09:00:00Z",
        generated_by: "deterministic",
        sentences: [],
      }),
  };
}

describe("DealRoomTab", () => {
  it("draws the room's band once the deal has one", async () => {
    stubWithSession(roomRoutes(), { deal_room: ["read", "update"] });
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    expect(
      await screen.findByRole("heading", {
        name: "Acme Expansion — Deal Room",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(translate("en", "roomtab.welcomeEmpty")),
    ).toBeInTheDocument();
  });

  it("offers to open a room when the deal has none", async () => {
    stubWithSession(
      { "GET /deal-rooms": () => jsonResponse({ data: [], page: {} }) },
      { deal_room: ["read", "create"] },
    );
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    expect(
      await screen.findByRole("button", { name: "Open a Deal Room" }),
    ).toBeInTheDocument();
  });

  it("a reader who may not create is offered no way to open a room", async () => {
    stubWithSession(
      { "GET /deal-rooms": () => jsonResponse({ data: [], page: {} }) },
      { deal_room: ["read"] },
    );
    const { container } = withProviders(
      <DealRoomTab dealId="deal-1" dealName="Acme Expansion" />,
    );

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});

describe("the tab carries the room's own access list", () => {
  // On the tab, not in the record's details pane: the pane is shut when a
  // reader arrives, so a room they had just opened offered them no way to let
  // anybody into it.
  it("names who may enter once a room exists", async () => {
    stubWithSession(
      {
        "GET /deal-rooms": () =>
          jsonResponse({ data: [room()], page: { next_cursor: null } }),
        "GET /deal-rooms/room-1/participants": () =>
          jsonResponse({ data: [], page: {} }),
      },
      { deal_room: ["read", "update"] },
    );
    withProviders(<DealRoomTab dealId="deal-1" dealName="BaymeOps" />);

    expect(
      await screen.findByText(translate("en", "access.title")),
    ).toBeInTheDocument();
  });
});

describe("Manage room opens the room's settings", () => {
  it("saves the text first and the access verb last", async () => {
    const calls: string[] = [];
    stubWithSession(
      {
        ...roomRoutes(),
        "PATCH /deal-rooms/room-1": (body) => {
          calls.push(`PATCH ${JSON.stringify(body)}`);
          return jsonResponse(room({ title: "Rollout", version: 2 }));
        },
        "POST /deal-rooms/room-1/pause": () => {
          calls.push("POST pause");
          return jsonResponse(room({ state: "paused", version: 3 }));
        },
      },
      { deal_room: ["read", "update"] },
    );
    const user = userEvent.setup();
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    await user.click(
      await screen.findByRole("button", { name: "Manage room" }),
    );
    const title = await screen.findByLabelText(/Room title/);
    await user.clear(title);
    await user.type(title, "Rollout");
    await user.click(screen.getByRole("radio", { name: /Paused/ }));
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(calls).toEqual([
        'PATCH {"title":"Rollout","welcome_message":null}',
        "POST pause",
      ]),
    );
  });

  it("warns before a room is closed", async () => {
    stubWithSession(roomRoutes(), { deal_room: ["read", "update"] });
    const user = userEvent.setup();
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    await user.click(
      await screen.findByRole("button", { name: "Manage room" }),
    );
    await user.click(await screen.findByRole("radio", { name: /Closed/ }));

    expect(
      screen.getByText(translate("en", "roompage.closeBody")),
    ).toBeInTheDocument();
  });

  it("resumes a paused room from its notice", async () => {
    const resumed = vi.fn(() => jsonResponse(room({ version: 2 })));
    stubWithSession(
      { ...roomRoutes("paused"), "POST /deal-rooms/room-1/resume": resumed },
      { deal_room: ["read", "update"] },
    );
    const user = userEvent.setup();
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    await user.click(await screen.findByRole("button", { name: "Resume" }));

    await waitFor(() => expect(resumed).toHaveBeenCalledTimes(1));
  });

  it("offers a reader who may not manage the room no way to its settings", async () => {
    stubWithSession(roomRoutes(), { deal_room: ["read"] });
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    expect(
      await screen.findByRole("heading", {
        name: "Acme Expansion — Deal Room",
      }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Manage room" })).toBeNull();
  });
});

describe("the tab says what the room still lacks", () => {
  it("counts the setup steps the room's records have done", async () => {
    stubWithSession(
      {
        ...roomRoutes(),
        "GET /deal-rooms": () =>
          jsonResponse({
            data: [room({ welcome_message: "Hello" })],
            page: { next_cursor: null },
          }),
      },
      { deal_room: ["read", "update"] },
    );
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    expect(
      await screen.findByText(translate("en", "roomtab.setup.title")),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        translate("en", "checklist.count", { done: "1", total: "4" }),
      ),
    ).toBeInTheDocument();
  });
});

describe("inviting from the setup list", () => {
  const AT = "2026-10-22T09:00:00Z";
  const seat = {
    id: "part-1",
    room_id: "room-1",
    full_name: "Dana Buyer",
    email: "dana@nordwind.example",
    capability: "comment",
    delivery_state: "delivered",
    has_signed_in: false,
    source: "manual",
    created_at: AT,
    updated_at: AT,
  };
  const issuedTitle = translate("en", "access.issued.title", {
    name: "Dana Buyer",
  });

  afterEach(() => focusManager.setFocused(undefined));

  // A room whose only open step is the invite. Once the invite is sent, the
  // seats read again with the new buyer in them.
  function stubLastStepRoom(roomReads: { failing: boolean }) {
    let invited = false;
    stubWithSession(
      {
        ...roomRoutes(),
        "GET /deal-rooms": () =>
          roomReads.failing
            ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
            : jsonResponse({
                data: [room({ welcome_message: "Hello" })],
                page: { next_cursor: null },
              }),
        "GET /deal-rooms/room-1/participants": () =>
          jsonResponse({ data: invited ? [seat] : [], page: {} }),
        "GET /deal-rooms/room-1/documents": () =>
          jsonResponse({
            data: [
              {
                id: "doc-1",
                room_id: "room-1",
                attachment_id: "att-1",
                group_key: "legal",
                title: "MSA redline",
                filename: "msa-redline.docx",
                reaches_buyer: true,
                position: 0,
                source: "manual",
                created_at: AT,
                updated_at: AT,
              },
            ],
            page: { next_cursor: null },
          }),
        "GET /deal-rooms/room-1/threads": () =>
          jsonResponse({
            data: [
              {
                id: "thread-1",
                room_id: "room-1",
                state: "open",
                required_change: false,
                author: { name: "Dana Buyer", side: "buyer" },
                created_at: AT,
                comments: [],
              },
            ],
            page: { next_cursor: null },
          }),
        "POST /deal-rooms/room-1/participants": () => {
          invited = true;
          return jsonResponse(
            {
              participant: seat,
              credential: "one-time-credential",
              credential_expires_at: "2026-11-22T09:00:00Z",
              queued: false,
            },
            201,
          );
        },
      },
      { deal_room: ["read", "update"] },
    );
  }

  async function sendTheInviteStep(user: ReturnType<typeof userEvent.setup>) {
    const step = (
      await screen.findByText(translate("en", "roomtab.setup.invite"))
    ).closest("li");
    if (!step) {
      throw new Error("the invite step is not a list item");
    }
    await user.click(
      within(step).getByRole("button", {
        name: translate("en", "access.invite"),
      }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.type(
      within(dialog).getByLabelText(translate("en", "access.nameLabel"), {
        exact: false,
      }),
      "Dana Buyer",
    );
    await user.type(
      within(dialog).getByLabelText(translate("en", "access.emailLabel"), {
        exact: false,
      }),
      "dana@nordwind.example",
    );
    await user.click(
      within(dialog).getByRole("button", {
        name: translate("en", "access.inviteConfirm"),
      }),
    );
    expect(await screen.findByText(issuedTitle)).toBeInTheDocument();
  }

  // The invite is the last step, so the list goes away once the seats are read
  // again. The one-time link must outlive it.
  it("keeps the issued link when the invite finishes the list", async () => {
    stubLastStepRoom({ failing: false });
    const user = userEvent.setup();
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    await sendTheInviteStep(user);

    await waitFor(() =>
      expect(
        screen.queryByText(translate("en", "roomtab.setup.title")),
      ).not.toBeInTheDocument(),
    );
    expect(screen.getByText(issuedTitle)).toBeInTheDocument();
  });

  // The rep leaves to paste the link, and coming back reads the room again.
  it("keeps the issued link when the room fails to read again", async () => {
    const roomReads = { failing: false };
    stubLastStepRoom(roomReads);
    const user = userEvent.setup();
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    await sendTheInviteStep(user);
    roomReads.failing = true;
    focusManager.setFocused(false);
    focusManager.setFocused(true);

    expect(
      await screen.findByText(translate("en", "common.error")),
    ).toBeInTheDocument();
    expect(screen.getByText(issuedTitle)).toBeInTheDocument();
  });
});

describe("the room brief", () => {
  it("draws each sentence with what it was written from", async () => {
    stubWithSession(
      {
        ...roomRoutes(),
        "GET /deal-rooms/room-1/participants": () =>
          jsonResponse({
            data: [
              {
                id: "p-1",
                room_id: "room-1",
                full_name: "Lena Vogt",
                email: "lena.vogt@example.com",
                capability: "comment",
                delivery_state: "delivered",
                has_signed_in: true,
                source: "manual",
                created_at: "2026-08-22T09:10:00Z",
                updated_at: "2026-08-24T14:02:00Z",
              },
            ],
            page: {},
          }),
        "GET /deal-rooms/room-1/brief": () =>
          jsonResponse({
            room_id: "room-1",
            generated_at: "2026-10-22T09:00:00Z",
            generated_by: "deterministic",
            sentences: [
              {
                text: "Lena Vogt asked for a new link.",
                nature: "fact",
                evidence: [{ entity_type: "participant", entity_id: "p-1" }],
              },
            ],
          }),
      },
      { deal_room: ["read", "update"] },
    );
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    expect(
      await screen.findByText("Lena Vogt asked for a new link."),
    ).toBeInTheDocument();
    expect(
      await screen.findByText(
        translate("en", "roomtab.brief.from", { sources: "Lena Vogt" }),
      ),
    ).toBeInTheDocument();
  });

  it("draws nothing for a room with nothing to say", async () => {
    stubWithSession(roomRoutes(), { deal_room: ["read", "update"] });
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    await screen.findByRole("heading", { name: "Acme Expansion — Deal Room" });
    await waitFor(() =>
      expect(
        screen.queryByText(translate("en", "roomtab.brief.title")),
      ).toBeNull(),
    );
  });
});

describe("a room buyers cannot simply walk into says so", () => {
  it.each([
    ["closed", "roompage.banner.closed"],
    ["expired", "roompage.banner.expired"],
    ["archived", "roompage.banner.archived"],
  ] as const)(
    "names a %s room's state, with the way to its settings",
    async (state, key) => {
      stubWithSession(roomRoutes(state), { deal_room: ["read", "update"] });
      withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

      expect(await screen.findByText(translate("en", key))).toBeInTheDocument();
      expect(
        screen.getAllByRole("button", { name: "Manage room" }).length,
      ).toBeGreaterThan(1);
    },
  );
});

describe("the settings dialog's access changes", () => {
  it("sets the end date before it closes the room", async () => {
    const calls: string[] = [];
    stubWithSession(
      {
        ...roomRoutes(),
        "PUT /deal-rooms/room-1/expiry": (body) => {
          calls.push(`PUT ${JSON.stringify(body)}`);
          return jsonResponse(room({ version: 2 }));
        },
        "POST /deal-rooms/room-1/close": () => {
          calls.push("POST close");
          return jsonResponse(room({ state: "closed", version: 3 }));
        },
      },
      { deal_room: ["read", "update"] },
    );
    const user = userEvent.setup();
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    await user.click(
      await screen.findByRole("button", { name: "Manage room" }),
    );
    await user.type(
      await screen.findByRole("textbox", { name: /Access ends on/ }),
      "31/12/2026{Enter}",
    );
    await user.click(screen.getByRole("radio", { name: /Closed/ }));
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(calls).toEqual([
        'PUT {"expires_at":"2026-12-31T23:59:59.000Z"}',
        "POST close",
      ]),
    );
  });
});

describe("the brief follows the room", () => {
  it("is written again after a thread is posted", async () => {
    const briefReads = vi.fn(() =>
      jsonResponse({
        room_id: "room-1",
        generated_at: "2026-10-22T09:00:00Z",
        generated_by: "deterministic",
        sentences: [],
      }),
    );
    stubWithSession(
      {
        ...roomRoutes(),
        "GET /deal-rooms/room-1/brief": briefReads,
        "POST /deal-rooms/room-1/threads": () =>
          jsonResponse({ id: "th-1", room_id: "room-1", comments: [] }),
      },
      { deal_room: ["read", "update"] },
    );
    const user = userEvent.setup();
    withProviders(<DealRoomTab dealId="deal-1" dealName="Acme Expansion" />);

    await user.type(
      await screen.findByLabelText("New thread"),
      "Is the go-live date fixed?",
    );
    await user.click(screen.getByRole("button", { name: "Post" }));

    await waitFor(() => expect(briefReads).toHaveBeenCalledTimes(2));
  });
});
