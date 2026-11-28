// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { pickOption } from "../../../design-system/select-testing";
import { en } from "../../../i18n/en";
import { dealsQueryParams } from "../../../lib/deals.queries";
import { steppedClock } from "../../../testing/steppedclock";
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
    removable: true,
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
    removable: false,
  },
];

const linkTypes = [
  { name: "renews", label: "Renews", from: ["deal"], to: ["deal"] },
  {
    name: "related_to",
    label: "Related to",
    from: ["deal"],
    to: ["deal", "company", "contact"],
  },
  {
    name: "ext:finance-demo:follow_up_of",
    label: "Follow-up of",
    unit: "finance-demo",
    from: ["company"],
    to: ["deal"],
  },
];

const hits = [
  { type: "deal", id: DEAL, title: "Warehouse rollout" },
  { type: "deal", id: RENEWAL, title: "Warehouse renewal" },
  { type: "contact", id: ORIGINAL, title: "Bob Buyer" },
];

function mount(writable: boolean, rows = links, types = linkTypes) {
  const removed: string[] = [];
  installFetchStub({
    "GET /search": () =>
      jsonResponse({ data: hits, page: { has_more: false } }),
    "POST /record-links": () =>
      jsonResponse(
        { title: "A record cannot be linked to itself", status: 422 },
        422,
      ),
    "GET /me": meRoute({ deal: ["read", "update"] } as never),
    "GET /record-links": () => jsonResponse({ data: rows }),
    "GET /link-types": () => jsonResponse({ data: types }),
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
      await screen.findByText("Deal · From this deal: Renews"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Company · To this deal: Follow-up of"),
    ).toBeInTheDocument();
  });

  it("names a type no unit declares with a neutral word, never its key", async () => {
    mount(true, links, []);
    expect(
      await screen.findByText(
        `Company · To this deal: ${en["deal.links.unknownType"]}`,
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/ext:/)).toBeNull();
  });

  it("names each hit's kind and leaves the deal itself out", async () => {
    const user = steppedClock();
    mount(true);
    await user.click(
      await screen.findByRole("button", { name: en["deal.links.add"] }),
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: en["deal.links.type"] }),
      "Related to",
    );
    await user.type(
      screen.getByRole("searchbox", { name: en["deal.links.search"] }),
      "Wa",
    );
    const options = await screen.findAllByRole("button", {
      name: /^(Warehouse|Bob)/,
    });
    expect(options.map((option) => option.textContent)).toEqual([
      "Warehouse renewalDeal",
      "Bob BuyerContact",
    ]);
  });

  it("starts a new link without the last one's refusal", async () => {
    const user = steppedClock();
    mount(true);
    await user.click(
      await screen.findByRole("button", { name: en["deal.links.add"] }),
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: en["deal.links.type"] }),
      "Related to",
    );
    await user.type(
      screen.getByRole("searchbox", { name: en["deal.links.search"] }),
      "Bo",
    );
    await user.click(await screen.findByRole("button", { name: /^Bob Buyer/ }));
    const refusal = await screen.findByText(/linked to itself/);
    await user.click(
      screen.getByRole("button", { name: en["deal.links.cancel"] }),
    );
    await user.click(
      screen.getByRole("button", { name: en["deal.links.add"] }),
    );
    expect(refusal).not.toBeInTheDocument();
  });

  it("links each record under its own address, not the deal's", async () => {
    history.replaceState(null, "", `/r/deals/${DEAL}#/deals/${DEAL}`);
    mount(true);
    const renewal = await screen.findByRole("link", {
      name: "Warehouse renewal",
    });
    expect(renewal).toHaveAttribute(
      "href",
      `/r/deals/${RENEWAL}#/deals/${RENEWAL}`,
    );
    expect(screen.getByRole("link", { name: "Acme" })).toHaveAttribute(
      "href",
      `/r/companies/${ORIGINAL}#/companies/${ORIGINAL}`,
    );
    history.replaceState(null, "", "/");
  });

  it("removes only the links the server marks removable", async () => {
    const { removed } = mount(true);
    const buttons = await screen.findAllByRole("button", {
      name: en["deal.links.remove"],
    });
    expect(buttons).toHaveLength(1);
    await userEvent.click(buttons[0]);
    await screen.findByText("Warehouse renewal");
    expect(removed).toEqual([OUT_LINK]);
    expect(
      screen.getByRole("button", { name: en["deal.links.add"] }),
    ).toBeInTheDocument();
  });

  it("offers no way to add or remove when the deal is not the reader's to change", async () => {
    mount(
      false,
      links.map((link) => ({ ...link, removable: false })),
    );
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
