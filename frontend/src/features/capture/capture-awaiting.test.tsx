/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { translate } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { openRowMenu } from "../../testing/tablerow.testkit";
import { AwaitingContacts } from "./capture-awaiting";

type Sent = { method: string; path: string; ifMatch: string | null };

const ID = "0190b7a2-0000-7000-8000-000000000012";

const UNSURE = {
  contact_id: ID,
  display_name: "Marta Ortiz",
  emails: ["m.ortiz@steireif.de"],
  addresses: [{ email: "m.ortiz@steireif.de", state: "unsure" as const }],
  captured_at: "2026-09-10T14:40:00Z",
};

// Every write the drawer sent, and the contact read archiving guards on.
function stubRoutes(failWrites = false) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const path = decodeURIComponent(
        new URL(request.url, "https://test.local").pathname.replace(
          /^\/v1/,
          "",
        ),
      );
      if (request.method !== "GET") {
        sent.push({
          method: request.method,
          path,
          ifMatch: request.headers.get("If-Match"),
        });
        return failWrites
          ? jsonResponse({ title: "Internal Server Error" }, 500)
          : new Response(null, { status: 204 });
      }
      if (path === "/me") return jsonResponse(meFixture({ roles: ["rep"] }));
      if (path === `/contacts/${ID}`)
        return jsonResponse({ id: ID, version: 7 });
      return jsonResponse({ data: [] });
    }),
  );
  return sent;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("says whose move it is and why", () => {
  stubRoutes();
  render(
    <AwaitingContacts
      rows={[
        UNSURE,
        {
          ...UNSURE,
          contact_id: "0190b7a2-0000-7000-8000-000000000013",
          display_name: "Jo Brandt",
          addresses: [{ email: "m.ortiz@steireif.de", state: "classifying" }],
        },
      ]}
    />,
  );
  expect(
    screen.getByText(
      `${en["senders.waiting.onYou"]} ${en["senders.waiting.why.unsure"]}`,
    ),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      `${en["senders.waiting.onMargince"]} ${en["senders.waiting.why.classifying"]}`,
    ),
  ).toBeInTheDocument();
});

it("marks the sender as business and shares the contact, after saying so", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes();
  render(<AwaitingContacts rows={[UNSURE]} />);

  const menu = await openRowMenu(user, "Marta Ortiz");
  await user.click(
    menu.getByRole("button", { name: en["senders.markBusiness"] }),
  );
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.getByText(en["senders.waiting.businessBody"])).toBeVisible();
  expect(sent).toEqual([]);

  await user.click(
    dialog.getByRole("button", { name: en["senders.markBusiness"] }),
  );
  await waitFor(() =>
    expect(sent.map((s) => `${s.method} ${s.path}`)).toEqual([
      "PUT /capture/senders/m.ortiz@steireif.de/decision",
      `POST /contacts/${ID}/publish`,
    ]),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

it("changes nothing when the reader cancels", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes();
  render(<AwaitingContacts rows={[UNSURE]} />);

  const menu = await openRowMenu(user, "Marta Ortiz");
  await user.click(menu.getByRole("button", { name: en["senders.keepOut"] }));
  await user.click(
    within(await screen.findByRole("dialog")).getByRole("button", {
      name: en["create.cancel"],
    }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(sent).toEqual([]);
});

// An answer is about one address, so a contact with two asks which.
it("answers about the address the reader picked", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes();
  render(
    <AwaitingContacts
      rows={[
        {
          ...UNSURE,
          emails: ["m.ortiz@steireif.de", "marta@private.example"],
          addresses: [
            { email: "m.ortiz@steireif.de", state: "unsure" },
            { email: "marta@private.example", state: "unjudgeable" },
          ],
        },
      ]}
    />,
  );

  const menu = await openRowMenu(user, "Marta Ortiz");
  await user.click(
    menu.getByRole("button", {
      name: translate("en", "senders.waiting.keepOutFor", {
        address: "marta@private.example",
      }),
    }),
  );
  await user.click(
    within(await screen.findByRole("dialog")).getByRole("button", {
      name: en["senders.keepOutConfirm"],
    }),
  );
  await waitFor(() =>
    expect(sent.map((s) => `${s.method} ${s.path}`)).toEqual([
      "PUT /capture/senders/marta@private.example/decision",
    ]),
  );
});

it("removes the contact without deciding the sender, on the version it read", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes();
  render(<AwaitingContacts rows={[UNSURE]} />);

  const menu = await openRowMenu(user, "Marta Ortiz");
  await user.click(
    menu.getByRole("button", { name: en["senders.waiting.remove"] }),
  );
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.getByText(en["senders.waiting.removeBody"])).toBeVisible();
  await user.click(
    dialog.getByRole("button", { name: en["senders.waiting.removeConfirm"] }),
  );
  await waitFor(() =>
    expect(sent).toEqual([
      { method: "DELETE", path: `/contacts/${ID}`, ifMatch: "7" },
    ]),
  );
});

it("keeps the dialog and says so when the answer fails", async () => {
  const user = userEvent.setup();
  stubRoutes(true);
  render(<AwaitingContacts rows={[UNSURE]} />);

  const menu = await openRowMenu(user, "Marta Ortiz");
  await user.click(
    menu.getByRole("button", { name: en["senders.markBusiness"] }),
  );
  const dialog = within(await screen.findByRole("dialog"));
  await user.click(
    dialog.getByRole("button", { name: en["senders.markBusiness"] }),
  );
  expect(await dialog.findByRole("alert")).toBeInTheDocument();
  expect(screen.getByRole("dialog")).toBeInTheDocument();
});

it("offers no answer again for an address already answered", async () => {
  const user = userEvent.setup();
  stubRoutes();
  render(
    <AwaitingContacts
      rows={[
        {
          ...UNSURE,
          addresses: [{ email: "m.ortiz@steireif.de", state: "decided" }],
        },
      ]}
    />,
  );
  const menu = await openRowMenu(user, "Marta Ortiz");
  expect(
    menu.queryByRole("button", { name: en["senders.markBusiness"] }),
  ).toBeNull();
  expect(
    menu.getByRole("button", { name: en["senders.waiting.remove"] }),
  ).toBeInTheDocument();
});
