// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { type WriteTo, WriteToProvider } from "../../lib/writeto";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { DealRoomAccess } from "./dealroomaccess";
import { InviteDialog } from "./dealroomaccess.dialogs";
import {
  MAIL_POLL_MS,
  MAIL_WATCH_MS,
  seatPollInterval,
} from "./dealroomaccess.queries";

type DealRoom = components["schemas"]["DealRoom"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ROOM: DealRoom = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Acme Expansion — Deal Room",
  state: "live",
  source: "manual",
  captured_by: "u-me",
  version: 1,
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-22T09:00:00Z",
};

type Participant = components["schemas"]["DealRoomParticipant"];

function drawWithDownloads(
  downloads: number,
  seat: Partial<Participant> = {},
  writeTo: WriteTo | null = null,
) {
  installFetchStub({
    "GET /deal-rooms/room-1/participants": () =>
      jsonResponse({
        data: [
          {
            id: "part-1",
            room_id: ROOM.id,
            full_name: "Dana Buyer",
            email: "dana.buyer@brandt-automotive.example",
            capability: "read",
            delivery_state: "delivered",
            has_signed_in: true,
            source: "manual",
            captured_by: "u-me",
            created_at: "2026-08-22T09:10:00Z",
            updated_at: "2026-08-24T14:02:00Z",
            download_count: downloads,
            ...seat,
          },
        ],
        page: { next_cursor: null },
      }),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <WriteToProvider writeTo={writeTo}>
          <DealRoomAccess room={ROOM} mayManage={false} />
        </WriteToProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("what a seat has taken out of the room", () => {
  it.each([
    [1, "1 document downloaded"],
    [3, "3 documents downloaded"],
  ])(
    "counts the downloads in the reader's grammar (%i)",
    async (count, said) => {
      drawWithDownloads(count);
      expect(await screen.findByText(new RegExp(said))).toBeTruthy();
    },
  );

  it("names the documents taken on the seat's own line", async () => {
    drawWithDownloads(2, { documents_downloaded: ["Offer.pdf", "MSA.pdf"] });
    expect(
      await screen.findByText(/2 documents downloaded: Offer\.pdf, MSA\.pdf/),
    ).toBeTruthy();
  });
});

describe("where a seat stands", () => {
  it.each<[string, Partial<Participant>, string]>([
    ["signed in", {}, "Signed in"],
    ["not in yet", { has_signed_in: false }, "Invited"],
    [
      "asking for a new link",
      { link_requested_at: "2026-08-25T08:00:00Z" },
      "Link requested",
    ],
    ["revoked", { revoked_at: "2026-08-26T08:00:00Z" }, "Revoked"],
    [
      "waiting on a mail that failed",
      { has_signed_in: false, delivery_state: "failed" },
      "Email failed",
    ],
  ])("badges a seat that is %s", async (_case, seat, badge) => {
    drawWithDownloads(0, seat);
    expect(await screen.findByText(badge)).toBeTruthy();
  });
});

describe("a buyer's ask for a new link", () => {
  it("stops asking the seller once the buyer is back in", async () => {
    drawWithDownloads(0, {
      link_requested_at: "2026-08-25T08:00:00Z",
      last_seen_at: "2026-08-25T09:00:00Z",
    });
    expect(await screen.findByText("Signed in")).toBeTruthy();
    expect(screen.queryByText(/Asked for a new link/)).toBeNull();
  });
});

describe("what became of the invitation mail", () => {
  it("tells the seller a failed mail is theirs to send", async () => {
    drawWithDownloads(0, { has_signed_in: false, delivery_state: "failed" });
    expect(
      await screen.findByText(
        "The invitation email failed. Issue a new link and send it yourself.",
      ),
    ).toBeTruthy();
  });

  it("says the mail was sent once the relay took it", async () => {
    drawWithDownloads(0, { has_signed_in: false, delivery_state: "sent" });
    expect(await screen.findByText(/invited · email sent/)).toBeTruthy();
  });
});

describe("reading the roster again while a mail is on its way", () => {
  const pending = [{ delivery_state: "pending" as const, revoked_at: null }];
  const now = 1_000_000;

  it("polls while a queued mail has no answer yet", () => {
    expect(seatPollInterval(pending, now + MAIL_WATCH_MS, now)).toBe(
      MAIL_POLL_MS,
    );
  });

  it("does not poll when no invitation was queued here", () => {
    expect(seatPollInterval(pending, undefined, now)).toBe(false);
  });

  it("stops once the send budget has passed, since pending also means no relay", () => {
    expect(seatPollInterval(pending, now - 1, now)).toBe(false);
  });

  it("stops once the relay's answer is on every row", () => {
    expect(
      seatPollInterval(
        [{ delivery_state: "failed", revoked_at: null }],
        now + MAIL_WATCH_MS,
        now,
      ),
    ).toBe(false);
  });
});

describe("a seat's address", () => {
  it("opens the composer filed under the room's deal", async () => {
    const writeTo = vi.fn();
    drawWithDownloads(0, {}, writeTo);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "dana.buyer@brandt-automotive.example",
      }),
    );
    expect(writeTo).toHaveBeenCalledWith({
      entityType: "deal",
      entityId: "deal-1",
      address: "dana.buyer@brandt-automotive.example",
    });
  });
});

function drawInvite(
  answer: () => Response,
  locale: "en" | "de" | "vi" = "en",
): { posted: unknown[] } {
  const posted: unknown[] = [];
  installFetchStub({
    "POST /deal-rooms/room-1/participants": (body) => {
      posted.push(body);
      return answer();
    },
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <InviteDialog room={ROOM} open onClose={() => {}} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { posted };
}

describe("the invite form", () => {
  it("refuses an address that is not one on the Email field, before sending", async () => {
    const user = userEvent.setup();
    const { posted } = drawInvite(() => jsonResponse({}));
    await user.type(screen.getByLabelText(/^Name/), "Dana Buyer");
    const email = screen.getByLabelText(/^Email/);
    await user.type(email, "not-an-email");
    await user.click(screen.getByRole("button", { name: "Invite" }));

    expect(
      await screen.findByText("Enter an email address for Email."),
    ).toBeTruthy();
    expect(email.getAttribute("aria-invalid")).toBe("true");
    await waitFor(() => expect(document.activeElement).toBe(email));
    expect(posted).toEqual([]);

    await user.type(email, "@example.com");
    expect(screen.queryByText("Enter an email address for Email.")).toBeNull();
  });

  it("says which empty field Invite needs", async () => {
    const user = userEvent.setup();
    drawInvite(() => jsonResponse({}));
    await user.click(screen.getByRole("button", { name: "Invite" }));
    expect(await screen.findByText("Name is required.")).toBeTruthy();
    expect(screen.getByText("Email is required.")).toBeTruthy();
  });

  it("puts the server's refusal on the field it names, by its label", async () => {
    const user = userEvent.setup();
    drawInvite(() =>
      jsonResponse(
        {
          code: "validation_error",
          detail: "`email` must be an email address",
          details: {
            errors: [
              {
                field: "email",
                code: "email_malformed",
                message: "`email` must be an email address",
              },
            ],
          },
        },
        422,
      ),
    );
    await user.type(screen.getByLabelText(/^Name/), "Dana Buyer");
    await user.type(screen.getByLabelText(/^Email/), "dana@example.c0m");
    await user.click(screen.getByRole("button", { name: "Invite" }));

    expect(
      await screen.findByText("Enter an email address for Email."),
    ).toBeTruthy();
    expect(screen.queryByText(/`email`/)).toBeNull();
  });
});
