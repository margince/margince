// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// A bounced message names a customer who did not receive something.
//
// It is the one health row with a contact on the other end of it, and the
// reader's move is to fix the address and send again. Neither happens here —
// both live on the contact's page — so what this row owes the reader is a way
// THERE, and the defect it must not have is a row that reports the failure and
// offers nothing.

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  day,
  echoView,
  jsonResponse,
  renderWorklist,
  row,
  stub,
} from "../../testing/worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a bounced message is reachable", () => {
  it("routes the reader to the contact whose address failed", async () => {
    stub(
      day({
        queue: [
          row({
            id: "bounce-1",
            source: "bounce",
            title: "Retrofit quote",
            detail: "The address does not exist.",
            actions: ["open"],
            subject: { type: "contact", id: "p-1", label: "Kirsten Bauer" },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );

    renderWorklist();

    // The TITLE is the route: the copy layer names the send and the contact it
    // failed for, and links the pair at the record where the address is fixed.
    // A row that reported the bounce and offered nothing would carry the same
    // words as plain text, so the assertion is the href rather than the name.
    const link = await screen.findByRole("link", {
      name: /Retrofit quote/,
    });
    expect(link.getAttribute("href")).toContain("p-1");
  });

  it("still draws the row when no contact is named, without a verb it cannot honour", async () => {
    stub(
      day({
        queue: [
          row({
            id: "bounce-2",
            source: "bounce",
            title: "Retrofit quote",
            detail: "The address does not exist.",
            actions: [],
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );

    renderWorklist();

    // The failure is still reported — a bounce nobody could file under a contact
    // is still a customer who did not receive something, and dropping the row
    // would hide it. What it must NOT do is offer a way somewhere it cannot go.
    expect(await screen.findByText("The address does not exist.")).toBeTruthy();
    expect(screen.queryByRole("link", { name: /Retrofit quote/ })).toBeNull();
  });
});

// "Send again" is the one verb a bounce row draws itself. The server offers it
// only on a bounce that may be sent once more, so the row draws it exactly
// then, and the reader's typed address is all the dialog asks for.
describe("a bounced message can be sent again", () => {
  const bounce = (actions: ("open" | "send_again")[]) =>
    row({
      id: "bounce-1",
      source: "bounce",
      title: "Retrofit quote",
      detail: "old@buyer.test: The address does not exist.",
      actions,
      subject: { type: "contact", id: "p-1", label: "Kirsten Bauer" },
    });

  // The queue as the server draws it before and after the send: the row
  // leaves once the repeat is accepted, as it would on the refetch.
  function stubSendAgain(answer: Response) {
    let sent: { url: string; body: unknown } | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const target =
          input instanceof Request ? input : new Request(input, init);
        if (
          target.url.includes("/attention/bounces/") &&
          target.method === "POST"
        ) {
          sent = { url: target.url, body: await target.json() };
          return answer.clone();
        }
        if (target.url.includes("/worklist")) {
          const queue =
            sent && answer.ok ? [] : [bounce(["open", "send_again"])];
          return jsonResponse(
            echoView(
              target.url,
              day({
                queue,
                summary: {
                  urgent: 0,
                  due: 0,
                  lower_priority: queue.length,
                  total: queue.length,
                },
              }),
            ),
          );
        }
        return jsonResponse({ data: [] });
      }),
    );
    return () => sent;
  }

  it("posts the typed address to the bounce and takes the row off the list", async () => {
    const sent = stubSendAgain(jsonResponse({ id: "a-2" }));
    renderWorklist();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Send again" }));
    await user.type(
      await screen.findByRole("textbox", { name: "Corrected address" }),
      "new@buyer.test",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(sent()).toBeDefined());
    expect(sent()?.url).toContain("/attention/bounces/bounce-1/send-again");
    expect(sent()?.body).toEqual({ address: "new@buyer.test" });
    // The queue is read again, and the answered row is gone from it.
    await waitFor(() =>
      expect(screen.queryByRole("link", { name: /Retrofit quote/ })).toBeNull(),
    );
  });

  it("shows the send's own refusal on the address and keeps the dialog open", async () => {
    stubSendAgain(
      new Response(
        JSON.stringify({
          title: "No consent",
          detail: "This contact has not agreed to receive this mail.",
        }),
        {
          status: 409,
          headers: { "content-type": "application/problem+json" },
        },
      ),
    );
    renderWorklist();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Send again" }));
    await user.type(
      await screen.findByRole("textbox", { name: "Corrected address" }),
      "new@buyer.test",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText(/has not agreed to receive/)).toBeTruthy();
    expect(
      screen.getByRole("textbox", { name: "Corrected address" }),
    ).toBeTruthy();
  });

  it("draws no button on a bounce that was already sent again", async () => {
    stub(
      day({
        queue: [bounce(["open"])],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    expect(
      await screen.findByRole("link", { name: /Retrofit quote/ }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Send again" })).toBeNull();
  });
});
