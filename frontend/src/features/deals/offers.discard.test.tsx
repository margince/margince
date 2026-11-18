/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithQueryClient } from "../../testing/queryrender";
import { jsonResponse } from "../../testing/story-utils";
import type { Offer } from "./offers";
import { OfferScreen } from "./offers";

// A draft can be discarded, and a discarded (archived) offer reads as archived
// with nothing left to change.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.location.hash = "";
});

const AT = "2026-10-01T09:00:00Z";

const DRAFT: Offer = {
  id: "o-1",
  deal_id: "d-1",
  offer_number: "A-1001",
  revision: 1,
  status: "draft",
  currency: "EUR",
  net_minor: 0,
  tax_minor: 0,
  gross_minor: 0,
  line_items: [],
  source: "manual",
  ai_generated: false,
  captured_by: "human:u-1",
  version: 4,
  created_at: AT,
  updated_at: AT,
};

type Archive = { ifMatch: string | null };

function serve(offer: Offer, archives: Archive[] = []) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const method = request ? request.method : (init?.method ?? "GET");
      const headers = request ? request.headers : new Headers(init?.headers);
      if (url.endsWith("/offers/o-1") && method === "DELETE") {
        archives.push({ ifMatch: headers.get("If-Match") });
        return jsonResponse({ ...offer, archived_at: AT });
      }
      if (url.endsWith("/offers/o-1")) return jsonResponse(offer);
      return jsonResponse({ data: [], page: { has_more: false } });
    }),
  );
  renderWithQueryClient(<OfferScreen id="o-1" />);
}

describe("discarding a draft offer", () => {
  it("archives the draft at the version read and goes back to the deal", async () => {
    const archives: Archive[] = [];
    serve(DRAFT, archives);
    const user = userEvent.setup();
    await user.click(await screen.findByTestId("archive-record"));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Discard draft",
      }),
    );
    await waitFor(() => expect(archives).toEqual([{ ifMatch: "4" }]));
    await waitFor(() => expect(globalThis.location.hash).toBe("#/deals/d-1"));
  });

  it("offers no discard once the offer is sent", async () => {
    serve({ ...DRAFT, status: "sent" });
    await screen.findByText("A-1001");
    expect(screen.queryByTestId("archive-record")).toBeNull();
  });
});

describe("an archived offer", () => {
  it("says it is archived and offers no way to change it", async () => {
    serve({ ...DRAFT, archived_at: AT });
    expect(await screen.findByText("Archived")).toBeTruthy();
    expect(screen.queryByText("Draft")).toBeNull();
    for (const control of [
      "edit-offer-header",
      "send-offer",
      "archive-record",
      "new-line-description",
      "render-pdf",
    ]) {
      expect(screen.queryByTestId(control)).toBeNull();
    }
    expect(screen.getByRole("heading", { name: "Line items" })).toBeTruthy();
  });
});
