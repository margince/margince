/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { meFixture } from "../../testing/mefixture";
import { TeamMarginceChangesPanel } from "./worklist.teamchanges";

type Team = components["schemas"]["TeamMarginceChanges"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const team: Team = {
  as_of: "2026-11-05T08:00:00Z",
  since: "2026-10-29T08:00:00Z",
  base_currency: "EUR",
  truncated: false,
  rows: [],
  groups: [
    {
      owner_id: "00000000-0000-4000-8000-0000000000a1",
      owner_name: "Anna Weber",
      kind: "deal_amount",
      deals: 1,
      undone: 0,
      value_base_minor: 4_500_000,
      value_moved_base_minor: 500_000,
      unvalued: 1,
    },
  ],
  ownerless_truncated: false,
  ownerless: [
    {
      deal_id: "00000000-0000-4000-8000-000000000001",
      deal_name: "Nobody's deal",
      reason: "no_owner",
      value_base_minor: 1_000_000,
    },
    {
      deal_id: "00000000-0000-4000-8000-000000000002",
      deal_name: "Left behind",
      reason: "departed_owner",
      owner_id: "00000000-0000-4000-8000-0000000000b2",
      owner_name: "Jonas Klein",
    },
    {
      deal_id: "00000000-0000-4000-8000-000000000003",
      deal_name: "Waiting on an invite",
      reason: "inactive_owner",
      owner_id: "00000000-0000-4000-8000-0000000000c3",
      owner_name: "Mia Roth",
    },
  ],
};

it("counts the value each kind of change moved, not the deals' value", async () => {
  stubApi({
    "GET /me": () => jsonResponse(meFixture({ allow: { deal: ["read"] } })),
    "GET /worklist/team/changes": () => jsonResponse(team),
  });
  render(<TeamMarginceChangesPanel enabled />);
  const changes = await screen.findByRole("table", {
    name: "Changes by Margince on your team",
  });
  expect(within(changes).getByText("Value moved")).toBeTruthy();
  expect(within(changes).getByText(/5,000/)).toBeTruthy();
  expect(within(changes).queryByText(/45,000/)).toBeNull();
  expect(screen.getByText(/Value moved leaves out changes/)).toBeTruthy();
});

it("lists the deals nobody can act for, each a link to give it an owner", async () => {
  stubApi({
    "GET /me": () => jsonResponse(meFixture({ allow: { deal: ["read"] } })),
    "GET /worklist/team/changes": () => jsonResponse(team),
  });
  render(<TeamMarginceChangesPanel enabled />);
  const ownerless = await screen.findByRole("table", {
    name: "Deals Margince leaves alone until someone owns them",
  });
  expect(within(ownerless).getByText("No owner")).toBeTruthy();
  expect(within(ownerless).getByText("Jonas Klein left")).toBeTruthy();
  expect(within(ownerless).getByText("Mia Roth is not active")).toBeTruthy();
  expect(
    within(ownerless).getByRole("link", { name: "Left behind" }),
  ).toBeTruthy();
});
