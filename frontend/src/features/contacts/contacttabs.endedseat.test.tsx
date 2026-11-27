/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { pretendReaderZone } from "../../testing/readerzone.testkit";
import { jsonResponse, stubWithSession } from "../../testing/story-utils";
import { ContactDealsTab } from "./contactdeals";

type Contact360 = components["schemas"]["Contact360"];
type Deal = components["schemas"]["Deal"];

const DEAL: Deal = {
  id: "d-1",
  name: "Fleet renewal 2026",
  pipeline_id: "pl-1",
  stage_id: "s-1",
  status: "open",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
};

beforeEach(() => {
  pretendReaderZone(UTC_ZONE);
  stubWithSession({ "GET /deals/d-1": () => jsonResponse(DEAL) }, {});
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// One seat on the deal above, current or ended as the case says.
function viewWithSeat(seatCurrent?: boolean): Contact360 {
  return {
    as_of: "2026-08-13T09:00:00Z",
    contact: {
      id: "p-1",
      full_name: "Dana Buyer",
      source: "manual",
      captured_by: "human:u-1",
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-08-01T08:00:00Z",
    },
    sections_omitted: [],
    deal_roles: {
      data: [
        {
          relationship_id: "r-1",
          deal_id: "d-1",
          deal_title: "Fleet renewal 2026",
          role: "economic_buyer",
          seat_current: seatCurrent,
        },
      ],
      page: { has_more: false },
    },
  };
}

function drawTab(view: Contact360) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactDealsTab view={view} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("an ended seat on the deals tab", () => {
  // A seat whose End day has passed stays listed as history. Both the card and
  // the row drawn before the deal is read say so.
  it("marks a seat that has ended, on the card and on the row", async () => {
    drawTab(viewWithSeat(false));
    expect(screen.getByText(/No longer on this deal/)).toBeTruthy();
    expect(
      await screen.findByText(englishCopy["contact.deals.seatEnded"]),
    ).toBeTruthy();
  });

  it("marks nothing on a current seat", () => {
    drawTab(viewWithSeat(true));
    expect(screen.queryByText(/No longer on this deal/)).toBeNull();
  });
});
