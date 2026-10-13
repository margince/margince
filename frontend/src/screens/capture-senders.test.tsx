/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { translate, translatePlural } from "../i18n";
import { en } from "../i18n/en";
import { CaptureSendersCard } from "./capture-senders";
import { jsonResponse, render } from "./settings.testkit";
import { openRowMenu } from "./tablerow.testkit";

type Sent = { method: string; path: string; body: unknown };

function stubRoutes(waiting: unknown[], senders: unknown[] = []) {
  const sent: Sent[] = [];
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
      if (path === "/capture/senders") return jsonResponse({ data: senders });
      if (path === "/capture/contacts-awaiting-decision")
        return jsonResponse({ data: waiting });
      if (path === "/me") return jsonResponse(meFixture({ roles: ["rep"] }));
      return jsonResponse({});
    }),
  );
  return sent;
}

const NEWSLETTER = {
  address: "news@letters.example",
  kind: "newsletter",
  overruled: false,
  record_exists: false,
};

const WAITING = {
  contact_id: "0190b7a2-0000-7000-8000-000000000012",
  display_name: "Marta Ortiz",
  emails: ["m.ortiz@steireif.de"],
  captured_at: "2026-09-10T14:40:00Z",
};

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

  render(<CaptureSendersCard />);

  expect(await screen.findByText("Waiting on a decision")).toBeInTheDocument();
  expect(await screen.findByText("Marta Ortiz")).toBeInTheDocument();
  // The first address names the sender; the rest are counted, not listed.
  const more = translatePlural("en", "senders.waiting.moreAddresses", 2, {
    count: "2",
  });
  expect(screen.getByText(`m.ortiz@steireif.de ${more}`)).toBeInTheDocument();
  expect(screen.getByText(en["senders.emptyBody"])).toBeInTheDocument();
});

it("opens the waiting contact from its row", async () => {
  const user = userEvent.setup();
  stubRoutes([WAITING]);

  render(<CaptureSendersCard />);

  await user.click(
    await screen.findByRole("button", {
      name: translate("en", "senders.waiting.openNamed", {
        name: "Marta Ortiz",
      }),
    }),
  );
  expect(globalThis.location.hash).toContain(WAITING.contact_id);
});

// A contact with no name and no address still needs a verb that says something.
it("names the open verb generically when the contact has no name", async () => {
  stubRoutes([{ ...WAITING, display_name: undefined, emails: [] }]);

  render(<CaptureSendersCard />);

  expect(
    await screen.findByRole("button", {
      name: en["senders.waiting.openUnnamed"],
    }),
  ).toBeInTheDocument();
});

it("draws no waiting group when nothing waits", async () => {
  stubRoutes([]);

  const { client } = render(<CaptureSendersCard />);

  expect(await screen.findByText(en["senders.emptyBody"])).toBeInTheDocument();
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
  stubRoutes([], [{ ...NEWSLETTER, kind: "export_control" }]);

  render(<CaptureSendersCard />);

  expect(await screen.findByText(en["senders.kind.other"])).toBeVisible();
  expect(screen.queryByText("export_control")).toBeNull();
});

it("marks a sender as business from its row's menu", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes([], [NEWSLETTER]);

  render(<CaptureSendersCard />);

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
});

// Excluding destroys mail already brought in, so it asks before it writes.
it("asks before excluding a sender", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes([], [NEWSLETTER]);

  render(<CaptureSendersCard />);

  const menu = await openRowMenu(user, NEWSLETTER.address);
  await user.click(menu.getByRole("button", { name: en["senders.keepOut"] }));
  const dialog = await screen.findByRole("dialog");
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

  render(<CaptureSendersCard />);

  expect(await screen.findByText(en["senders.byYou"])).toBeVisible();
  const menu = await openRowMenu(user, NEWSLETTER.address);
  await user.click(menu.getByRole("button", { name: en["senders.withdraw"] }));
  await waitFor(() =>
    expect(sent).toEqual([expect.objectContaining({ method: "DELETE" })]),
  );
});
