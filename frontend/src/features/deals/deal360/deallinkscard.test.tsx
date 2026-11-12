// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { en } from "../../../i18n/en";
import { dealsQueryParams } from "../../../lib/deals.queries";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealLinksCard } from "./deallinkscard";

const DEAL = "01a0e9f1-0000-7000-8000-0000000000a1";
const RENEWAL = "01a0e9f1-0000-7000-8000-0000000000a2";
const ORIGINAL = "01a0e9f1-0000-7000-8000-0000000000a3";
const OUT_LINK = "01a0e9f1-0000-7000-8000-0000000000b1";
const IN_LINK = "01a0e9f1-0000-7000-8000-0000000000b2";

const links = [
  {
    id: OUT_LINK,
    from_type: "deal",
    from_id: DEAL,
    from_name: "Warehouse rollout",
    to_type: "deal",
    to_id: RENEWAL,
    to_name: "Warehouse renewal",
    link_type: "renews",
    created_at: "2026-10-08T09:00:00Z",
  },
  {
    id: IN_LINK,
    from_type: "company",
    from_id: ORIGINAL,
    from_name: "Acme",
    to_type: "deal",
    to_id: DEAL,
    to_name: "Warehouse rollout",
    link_type: "ext:finance-demo:follow_up_of",
    created_at: "2026-10-09T09:00:00Z",
  },
];

const linkTypes = [
  { name: "renews", label: "Renews", from: ["deal"], to: ["deal"] },
  {
    name: "ext:finance-demo:follow_up_of",
    label: "Follow-up of",
    unit: "finance-demo",
    from: ["company"],
    to: ["deal"],
  },
];

function mount(writable: boolean) {
  const removed: string[] = [];
  installFetchStub({
    "GET /me": meRoute({ deal: ["read", "update"] } as never),
    "GET /record-links": () => jsonResponse({ data: links }),
    "GET /link-types": () => jsonResponse({ data: linkTypes }),
    [`DELETE /record-links/${OUT_LINK}`]: () => {
      removed.push(OUT_LINK);
      return new Response(null, { status: 204 });
    },
  });
  render(
    <StoryProviders>
      <DealLinksCard deal={{ id: DEAL, writable }} />
    </StoryProviders>,
  );
  return { removed };
}

afterEach(cleanup);

describe("DealLinksCard", () => {
  it("reads a link from either end, with the type's own label", async () => {
    mount(true);
    expect(await screen.findByText("Warehouse renewal")).toBeInTheDocument();
    expect(screen.getByText("Acme")).toBeInTheDocument();
    expect(
      await screen.findByText("From this deal: Renews"),
    ).toBeInTheDocument();
    expect(screen.getByText("To this deal: Follow-up of")).toBeInTheDocument();
  });

  it("removes a link when the reader may change the deal", async () => {
    const { removed } = mount(true);
    const [first] = await screen.findAllByRole("button", {
      name: en["deal.links.remove"],
    });
    await userEvent.click(first);
    await screen.findByText("Warehouse renewal");
    expect(removed).toEqual([OUT_LINK]);
    expect(
      screen.getByRole("button", { name: en["deal.links.add"] }),
    ).toBeInTheDocument();
  });

  it("offers no way to add or remove when the deal is not the reader's to change", async () => {
    mount(false);
    await screen.findByText("Warehouse renewal");
    expect(
      screen.queryByRole("button", { name: en["deal.links.add"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["deal.links.remove"] }),
    ).toBeNull();
  });
});

describe("dealsQueryParams", () => {
  const base = {
    pipelineId: "",
    q: "",
    sort: "",
    includeArchived: false,
  };

  it("sends the link type the reader chose", () => {
    expect(
      dealsQueryParams({ ...base, filters: { link_type: "renews" } }).link_type,
    ).toBe("renews");
  });

  it("sends none when no link type is chosen", () => {
    expect(
      dealsQueryParams({ ...base, filters: {} }).link_type,
    ).toBeUndefined();
  });
});
