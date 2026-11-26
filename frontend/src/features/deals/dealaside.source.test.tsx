// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { listsMe } from "../../testing/lists.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealAside } from "./dealaside";

// captured_by names only the agent's passport. The create audit row behind
// `created_by` is what knows the colleague and the tool.

type Deal = components["schemas"]["Deal"];

const AGENT = "agent:0b9f3c2a-6d7e-4c1b-9a8f-1e2d3c4b5a69";

const deal: Deal = {
  id: "d1",
  name: "Fleet retrofit",
  status: "open",
  writable: false,
  version: 1,
  pipeline_id: "p1",
  stage_id: "s1",
  source: "manual",
  captured_by: AGENT,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

afterEach(cleanup);

function mount(current: Deal) {
  installFetchStub({
    "GET /me": meRoute({ deal: ["read"] }, { seat: "full" }),
  });
  render(
    <StoryProviders>
      <DealAside deal={current} stages={[]} />
    </StoryProviders>,
  );
}

describe("the left column's Recorded by row on a deal an agent created", () => {
  it("names the colleague the agent worked for and the tool", async () => {
    mount({
      ...deal,
      created_by: {
        actor_type: "agent",
        actor_id: AGENT,
        on_behalf_of_name: "Anna Weber",
        agent_client: "Claude",
      },
    });

    expect(
      await screen.findByText("Made by Claude for Anna Weber"),
    ).toBeTruthy();
  });

  it("says only that an agent made it when the create row is not known", async () => {
    mount(deal);

    expect(await screen.findByText("Automated by an agent")).toBeTruthy();
  });
});

describe("the left column's Lists group", () => {
  function serve(onLists: unknown[] = []) {
    installFetchStub({
      "GET /me": listsMe(true, [], {
        allow: { deal: ["read"], list: ["read", "update"] },
      }),
      [`GET /records/deal/${deal.id}/lists`]: () =>
        jsonResponse({ data: onLists }),
    });
    render(
      <StoryProviders>
        <DealAside deal={deal} stages={[]} />
      </StoryProviders>,
    );
  }

  const group = async () =>
    within(
      await screen.findByRole("region", { name: en["lists.record.title"] }),
    );

  it("offers Add to Shortlist and every list while the deal is on none", async () => {
    serve();
    const lists = await group();
    expect(
      await lists.findByRole("button", { name: en["lists.addToShortlist"] }),
    ).toBeTruthy();
    expect(
      lists
        .getByRole("link", { name: en["lists.record.allLists"] })
        .getAttribute("href"),
    ).toBe("/#/filters/lists");
  });

  it("keeps the lists it is on beside the verbs", async () => {
    serve([{ id: "s-1", name: "Launch references", entity_type: "deal" }]);
    const lists = await group();
    expect(
      await lists.findByRole("link", { name: "Launch references" }),
    ).toBeTruthy();
    expect(
      lists.getByRole("button", { name: en["lists.addToShortlist"] }),
    ).toBeTruthy();
  });
});
