// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealAside } from "./dealaside";

type Deal = components["schemas"]["Deal"];

const deal: Deal = {
  id: "d1",
  name: "Fleet retrofit",
  status: "open",
  writable: false,
  version: 1,
  pipeline_id: "p1",
  stage_id: "s1",
  source: "manual",
  captured_by: "user:u-1",
  company_id: "c1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

afterEach(cleanup);

function mount(archivedAt: string | null) {
  installFetchStub({
    "GET /me": meRoute({ deal: ["read"], company: ["read"] }, { seat: "full" }),
    "GET /companies/c1": () =>
      jsonResponse({
        id: "c1",
        display_name: "Brandt Automotive GmbH",
        status: "customer",
        archived_at: archivedAt,
      }),
  });
  render(
    <StoryProviders>
      <DealAside deal={deal} stages={[]} />
    </StoryProviders>,
  );
}

describe("the account card on an open deal", () => {
  it("says the account is archived once it is", async () => {
    mount("2026-03-01T00:00:00Z");
    expect(await screen.findByText("Brandt Automotive GmbH")).toBeTruthy();
    expect(screen.getByText("Archived")).toBeTruthy();
  });

  it("shows the account's stage while it is live", async () => {
    mount(null);
    expect(await screen.findByText("Brandt Automotive GmbH")).toBeTruthy();
    expect(screen.queryByText("Archived")).toBeNull();
  });
});
