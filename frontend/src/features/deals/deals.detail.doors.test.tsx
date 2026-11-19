// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { translate } from "../../i18n";
import { DealScreen } from "./deals";
import { deal, jsonResponse, render, stubBackend } from "./deals.testkit";

type Activity = components["schemas"]["Activity"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

const EMAIL: Activity = {
  occurred_precision: "instant",
  offers_email: false,
  id: "a-in",
  kind: "email",
  direction: "inbound",
  subject: "Fleet renewal",
  occurred_at: "2026-08-02T09:00:00Z",
  is_done: true,
  source: "capture",
  captured_by: "system",
  created_at: "2026-08-02T09:00:00Z",
  updated_at: "2026-08-02T09:00:00Z",
};

// A deal with a newest email nobody answered, inside the page pane the doors
// scroll.
function openDeal() {
  const d = deal({ id: "x", version: 4 });
  const backend = stubBackend([d], { single: d });
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input instanceof Request ? input.url : input);
      if (
        new URL(url, "http://x").pathname.endsWith("/activities/latest-inbound")
      ) {
        return jsonResponse({
          data: [{ activity: EMAIL, kind: "email", accepts_request: false }],
        });
      }
      return backend(input, init);
    },
  );
  const view = render(
    <div className="scroll">
      <DealScreen id="x" />
    </div>,
  );
  const pane = view.container.querySelector<HTMLElement>(".scroll");
  if (!pane) throw new Error("the page pane was not drawn");
  const scrollTo = vi.fn();
  pane.scrollTo = scrollTo;
  return { scrollTo };
}

const tab = (name: string) =>
  screen.getByRole("button", { name: new RegExp(`^${name}`) });

describe("DealScreen — a link in a tab's body opens the other tab at its top", () => {
  it("opens the Deal Room at the top from the Overview's room card", async () => {
    const { scrollTo } = openDeal();
    await userEvent.click(
      await screen.findByRole("button", {
        name: translate("en", "deal.room.setUp"),
      }),
    );

    expect(tab(translate("en", "tab.dealRoom"))).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(scrollTo).toHaveBeenCalledWith(expect.objectContaining({ top: 0 }));
  });

  it("opens the Overview at the top from the newest email over another tab", async () => {
    const { scrollTo } = openDeal();
    await waitFor(() =>
      expect(tab(translate("en", "tab.history"))).toBeTruthy(),
    );
    await userEvent.click(tab(translate("en", "tab.history")));
    // The strip keeps the place rather than scrolling the pane to the top.
    expect(scrollTo).not.toHaveBeenCalled();

    await userEvent.click(
      await screen.findByRole("button", {
        name: translate("en", "latest.line.open", {
          kind: translate("en", "latest.kind.email"),
        }),
      }),
    );

    expect(tab(translate("en", "tab.overview"))).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(scrollTo).toHaveBeenCalledWith(expect.objectContaining({ top: 0 }));
  });
});
