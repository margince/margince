// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { recordNamesRoutes } from "../../testing/recordnames.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadQualifyPreviewCard } from "./leadcards";
import type { Lead } from "./leads";

afterEach(cleanup);

const promoted: Lead = {
  id: "l-1",
  full_name: "Dana Buyer",
  status: "promoted",
  score: 72,
  source: "manual",
  captured_by: "human:u-1",
  version: 2,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
  promoted_at: "2026-06-20T08:00:00Z",
  promoted_contact_id: "c-1",
};

const promotion = {
  outcome: "created",
  trigger: "human_qualify",
  pending: false,
  failed: false,
} as const;

function open(lead: Lead, employer: object | undefined) {
  installFetchStub({
    "GET /me": meRoute({
      contact: ["read"],
      company: ["read"],
      deal: ["read"],
    }),
    ...recordNamesRoutes({
      contact: { "c-1": "Dana Buyer" },
      company: { "co-1": "Pad AG" },
      deal: { "d-1": "Pad AG renewal" },
    }),
    "GET /contacts/c-1": () =>
      jsonResponse({ id: "c-1", full_name: "Dana Buyer", employer }),
  });
  render(
    <StoryProviders>
      <LeadQualifyPreviewCard lead={lead} promotion={promotion} />
    </StoryProviders>,
  );
}

describe("the Qualified as card", () => {
  it("lists the contact, the company and the deal qualifying made, each as a link", async () => {
    open(
      { ...promoted, qualified_deal_id: "d-1" },
      {
        company_id: "co-1",
        company_name: "Pad AG",
      },
    );
    expect(
      await screen.findByRole("link", { name: "Pad AG" }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("link", { name: "Pad AG renewal" }),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("link", { name: "Dana Buyer" }),
    ).toBeInTheDocument();
  });

  it("draws no company or deal row when qualifying made neither", async () => {
    open(promoted, undefined);
    await screen.findByRole("link", { name: "Dana Buyer" });
    expect(screen.queryByText("Company")).not.toBeInTheDocument();
    expect(screen.queryByText("Deal")).not.toBeInTheDocument();
  });
});
