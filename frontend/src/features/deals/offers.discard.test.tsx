/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RbacAction } from "../../lib/capability";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub } from "../../testing/story-utils";
import { jsonResponse, type Offer, offer, render } from "./deals.testkit";
import { OfferScreen } from "./offers";

// A draft can be discarded, and an archived offer leaves nothing to change.
// A refused step re-reads what the server holds.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.location.hash = "";
});

const AT = "2026-10-01T09:00:00Z";

const DRAFT = offer({
  id: "o-1",
  deal_id: "d-1",
  offer_number: "A-1001",
  version: 4,
});

type Archive = { ifMatch: string | null };

// Hand-rolled rather than installFetchStub, whose handlers never see the
// If-Match header the archive case is about.
function serve(
  draft: Offer,
  archives: Archive[] = [],
  offerGrants: readonly RbacAction[] = ["read", "delete"],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      const path = new URL(request.url).pathname;
      if (path === "/v1/me") {
        return jsonResponse(meFixture({ allow: { offer: offerGrants } }));
      }
      if (path.endsWith("/offers/o-1") && request.method === "DELETE") {
        archives.push({ ifMatch: request.headers.get("If-Match") });
        return jsonResponse({ ...draft, archived_at: AT });
      }
      if (path.endsWith("/offers/o-1")) return jsonResponse(draft);
      return jsonResponse({ data: [], page: { has_more: false } });
    }),
  );
  render(<OfferScreen id="o-1" />);
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

  it("is disabled for a seat that may not archive offers", async () => {
    serve(DRAFT, [], ["read", "update"]);
    await waitFor(() =>
      expect(
        screen.getByTestId("archive-record").hasAttribute("disabled"),
      ).toBe(true),
    );
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

describe("a refused lifecycle step", () => {
  it("re-reads the offer, so the page shows the server's status", async () => {
    let status: Offer["status"] = "sent";
    installFetchStub({
      "POST /offers/o-1/accept": () => {
        status = "superseded";
        return jsonResponse({ detail: "the offer is superseded" }, 422);
      },
      "GET /offers/o-1": () => jsonResponse({ ...DRAFT, status }),
      "GET /me": () => jsonResponse(meFixture({ allow: { offer: ["read"] } })),
    });
    render(<OfferScreen id="o-1" />);
    const user = userEvent.setup();
    await user.click(await screen.findByTestId("accept-offer"));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Accept",
      }),
    );
    expect(await screen.findByText("Superseded")).toBeTruthy();
    expect(screen.queryByTestId("accept-offer")).toBeNull();
  });
});
