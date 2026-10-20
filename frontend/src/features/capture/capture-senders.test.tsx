/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { translate, translatePlural } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { linkedInReachFixture } from "../../testing/reachfixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { openRowMenu } from "../../testing/tablerow.testkit";
import { ConnectionOutcomesCard } from "./connection-outcomes";

type Sent = { method: string; path: string; body: unknown };

// Every path the card read, for the cases that prove a read never happened.
const read: string[] = [];

function stubRoutes(waiting: unknown[], senders: unknown[] = []) {
  const sent: Sent[] = [];
  read.length = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(request.url, "https://test.local");
      const path = decodeURIComponent(url.pathname.replace(/^\/v1/, ""));
      if (request.method !== "GET") {
        const text = await request.clone().text();
        sent.push({
          method: request.method,
          path,
          body: text ? JSON.parse(text) : undefined,
        });
        return new Response(null, { status: 204 });
      }
      read.push(path);
      if (path === "/capture/senders") return jsonResponse({ data: senders });
      if (path === "/capture/contacts-awaiting-decision")
        return jsonResponse({ data: waiting });
      if (path === "/me") return jsonResponse(meFixture({ roles: ["rep"] }));
      if (path === "/capture/held-threads") return jsonResponse({ data: [] });
      if (path === "/me/linkedin-reach")
        return jsonResponse(linkedInReachFixture());
      return jsonResponse({});
    }),
  );
  return sent;
}

const CONTACT_ID = "0190b7a2-0000-7000-8000-000000000012";
const LAST_SEEN = "2026-09-10T14:40:00Z";

const NEWSLETTER = {
  address: "news@letters.example",
  kind: "newsletter",
  overruled: false,
  record_exists: false,
  message_count: 0,
};

const WAITING = {
  contact_id: CONTACT_ID,
  display_name: "Marta Ortiz",
  emails: ["m.ortiz@steireif.de"],
  captured_at: "2026-09-10T14:40:00Z",
};

// The lists open from the outcomes card, one drawer each.
async function openList(
  user: ReturnType<typeof userEvent.setup>,
  list: string,
) {
  render(<ConnectionOutcomesCard />);
  await user.click(
    await screen.findByRole("button", {
      name: translate("en", "connectionOutcomes.viewNamed", { name: list }),
    }),
  );
  return within(await screen.findByRole("dialog", { name: list }));
}

function settingRowOf(label: HTMLElement): HTMLElement {
  const row = label.closest<HTMLElement>(".settingrow");
  if (row === null) throw new Error("the label sits in no settings row");
  return row;
}

function rowOf(cell: HTMLElement): HTMLTableRowElement {
  const row = cell.closest("tr");
  if (row === null) throw new Error("the cell sits in no table row");
  return row;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The contact with no sender entry is the one the senders list cannot show,
// and the one the admin card counts for this mailbox all the same.
it("lists a waiting contact whose sender never reached the senders list", async () => {
  stubRoutes([
    {
      ...WAITING,
      emails: ["m.ortiz@steireif.de", "marta@steireif.de", "mo@steireif.de"],
    },
  ]);

  const user = userEvent.setup();
  const drawer = await openList(user, "Waiting on a decision");

  expect(drawer.getByText("Marta Ortiz")).toBeInTheDocument();
  // The first address names the sender; the rest are counted, not listed.
  const more = translatePlural("en", "senders.waiting.moreAddresses", 2, {
    count: "2",
  });
  expect(drawer.getByText(`m.ortiz@steireif.de ${more}`)).toBeInTheDocument();
});

it("opens the waiting contact from its row", async () => {
  const user = userEvent.setup();
  stubRoutes([WAITING]);

  const drawer = await openList(user, "Waiting on a decision");

  await user.click(
    drawer.getByRole("button", {
      name: translate("en", "senders.waiting.openNamed", {
        name: "Marta Ortiz",
      }),
    }),
  );
  expect(globalThis.location.hash).toContain(WAITING.contact_id);
});

// A contact with no name and no address still needs a verb that says something.
it("names the open verb generically when the contact has no name", async () => {
  const user = userEvent.setup();
  stubRoutes([{ ...WAITING, display_name: undefined, emails: [] }]);

  const drawer = await openList(user, "Waiting on a decision");

  expect(
    drawer.getByRole("button", { name: en["senders.waiting.openUnnamed"] }),
  ).toBeInTheDocument();
});

it("draws no waiting row when nothing waits, and offers no empty list", async () => {
  stubRoutes([]);

  const { client } = render(<ConnectionOutcomesCard />);

  const senders = settingRowOf(await screen.findByText(en["senders.title"]));
  expect(
    await within(senders).findByText(en["connectionOutcomes.none"]),
  ).toBeInTheDocument();
  expect(within(senders).queryByRole("button")).toBeNull();
  // Settled first, so the absence below is the empty answer and not a read
  // still in flight.
  await waitFor(() =>
    expect(
      client.getQueryState(["capture-contacts-awaiting-decision"])?.status,
    ).toBe("success"),
  );
  expect(screen.queryByText("Waiting on a decision")).not.toBeInTheDocument();
});

// The classifier's vocabulary moves on its own. A kind this build has no word
// for still reads as words, never as the server's token.
it("names a kind it has no word for in words", async () => {
  const user = userEvent.setup();
  stubRoutes([], [{ ...NEWSLETTER, kind: "export_control" }]);

  const drawer = await openList(user, en["senders.title"]);

  expect(drawer.getByText(en["senders.kind.other"])).toBeVisible();
  expect(drawer.queryByText("export_control")).toBeNull();
});

it("marks a sender as business from its row's menu", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes([], [NEWSLETTER]);

  await openList(user, en["senders.title"]);

  const menu = await openRowMenu(user, NEWSLETTER.address);
  await user.click(
    menu.getByRole("button", { name: en["senders.markBusiness"] }),
  );
  await waitFor(() =>
    expect(sent).toEqual([
      {
        method: "PUT",
        path: `/capture/senders/${NEWSLETTER.address}/decision`,
        body: { decision: "business" },
      },
    ]),
  );
  // The contacts that waited on this sender are settled with it.
  await waitFor(() =>
    expect(
      read.filter((path) => path === "/capture/contacts-awaiting-decision"),
    ).toHaveLength(2),
  );
});

// Excluding destroys mail already brought in, so it asks before it writes.
it("asks before excluding a sender", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes([], [NEWSLETTER]);

  await openList(user, en["senders.title"]);

  const menu = await openRowMenu(user, NEWSLETTER.address);
  await user.click(menu.getByRole("button", { name: en["senders.keepOut"] }));
  const dialog = await screen.findByRole("dialog", {
    name: en["senders.keepOutTitle"],
  });
  expect(sent).toEqual([]);
  await user.click(
    within(dialog).getByRole("button", { name: en["senders.keepOutConfirm"] }),
  );
  await waitFor(() =>
    expect(sent).toEqual([
      expect.objectContaining({ body: { decision: "keep_out" } }),
    ]),
  );
});

it("lets the owner withdraw their own answer, and says it is theirs", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes(
    [],
    [{ ...NEWSLETTER, decision: "business", overruled: true }],
  );

  const drawer = await openList(user, en["senders.title"]);

  expect(drawer.getByText(en["senders.byYou"])).toBeVisible();
  const menu = await openRowMenu(user, NEWSLETTER.address);
  await user.click(menu.getByRole("button", { name: en["senders.withdraw"] }));
  await waitFor(() =>
    expect(sent).toEqual([expect.objectContaining({ method: "DELETE" })]),
  );
});

// The contact a sender became is one press away, named by the list itself.
// No row reads its contact, and the count says how much mail there is.
it("links a sender to its contact beside its mail count and newest message", async () => {
  stubRoutes(
    [],
    [
      {
        ...NEWSLETTER,
        address: "m.ortiz@steireif.de",
        kind: "contact",
        record_exists: true,
        contact_id: CONTACT_ID,
        contact_name: "Marta Ortiz",
        message_count: 1204,
        last_seen_at: LAST_SEEN,
      },
    ],
  );

  const drawer = await openList(userEvent.setup(), en["senders.title"]);

  const link = drawer.getByRole("link", { name: "Marta Ortiz" });
  expect(link.getAttribute("href")).toContain(CONTACT_ID);
  const row = within(rowOf(link));
  expect(row.getByText("1,204")).toBeVisible();
  expect(
    row.getByText(formatDateTime(LAST_SEEN, "en", viewerZone())),
  ).toBeVisible();
  expect(read.filter((path) => path.startsWith("/contacts/"))).toEqual([]);
});

it("names no contact and no date for a sender with neither", async () => {
  stubRoutes([], [NEWSLETTER]);

  const drawer = await openList(userEvent.setup(), en["senders.title"]);

  const row = within(rowOf(drawer.getByText(NEWSLETTER.address)));
  expect(row.queryByRole("link")).toBeNull();
  expect(row.getAllByText("—")).toHaveLength(2);
  expect(row.getByText("0")).toBeVisible();
});

it("keeps a long address on one line and gives its whole text on focus", async () => {
  const generated = {
    ...NEWSLETTER,
    address:
      "bounces+srs=4f7a2c9e1b0d=mailer-relay-7731@notifications.example-mailer.com",
  };
  stubRoutes([], [generated]);
  const drawer = await openList(userEvent.setup(), en["senders.title"]);
  const address = drawer.getByText(generated.address);
  // happy-dom lays nothing out, so the clip the stylesheet makes is stated.
  vi.spyOn(address, "scrollWidth", "get").mockReturnValue(480);
  vi.spyOn(address, "clientWidth", "get").mockReturnValue(160);

  act(() => address.focus());

  expect(address).toHaveClass("capture-senders-sender");
  expect(await screen.findByRole("tooltip")).toHaveTextContent(
    generated.address,
  );
});
