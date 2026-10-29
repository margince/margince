/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { mount, view } from "../../testing/contactpage.testkit";
import { meFixture } from "../../testing/mefixture";
import { jsonResponse } from "../../testing/story-utils";

afterEach(cleanup);

// The reader's own way in, on the Network tab of the real page. The map offers
// the page's composer there, and only while the header may write to them.

const READER = meFixture({}).user.id;

const readerGraph = {
  contact_id: "p-1",
  nodes: [
    {
      id: "contact:p-1",
      type: "contact",
      group: "anchor",
      label: view.contact.full_name,
      contact_id: "p-1",
    },
    {
      id: `user:${READER}`,
      type: "colleague",
      group: "direct",
      label: "Demo Admin",
      user_id: READER,
    },
  ],
  edges: [
    {
      from: `user:${READER}`,
      to: "contact:p-1",
      strength_bucket: "strong",
      interactions_90d: 12,
    },
  ],
  routes: [
    {
      route_id: `direct:${READER}`,
      route_type: "direct",
      via_user_id: READER,
      via_display_name: "Demo Admin",
      strength_bucket: "strong",
      evidence: { interactions_90d: 12, two_way: true },
      availability: "available",
    },
  ],
  groups_omitted: [],
} as components["schemas"]["ContactGraph"];

const networkRoutes = {
  "GET /contacts/p-1/graph": () => jsonResponse(readerGraph),
  "GET /contacts/p-1/network": () =>
    jsonResponse({
      contact_id: "p-1",
      colleagues: [],
      capture_gaps: [],
      capture_gaps_total: 0,
    }),
  "GET /contacts/p-1/intro-requests": () => jsonResponse({ data: [] }),
};

async function readerNode(): Promise<HTMLElement> {
  return screen.findByRole("button", {
    name: new RegExp(`^${en["contact.intro.heroYou"]}, `),
  });
}

describe("the reader's own node on the contact page", () => {
  it("opens the page's composer from the map", async () => {
    const user = userEvent.setup();
    mount("network", view, [], networkRoutes);

    await user.click(await readerNode());
    // The address in the details carries the same verb; this one is the map's.
    const [onMap] = (
      await screen.findAllByRole("button", { name: en["compose.writeEmail"] })
    ).filter((button) => !button.closest(".record-aside"));
    if (!onMap) throw new Error("the map offered no write");
    await user.click(onMap);

    expect(
      await screen.findByRole("dialog", { name: /New email/ }),
    ).toBeTruthy();
  });

  it("offers no write while the header cannot write to them", async () => {
    const user = userEvent.setup();
    const unreachable = {
      ...view,
      contact: { ...view.contact, emails: [], primary_email: null },
    };
    mount("network", unreachable, [], networkRoutes);

    await user.click(await readerNode());
    expect(
      screen.queryByRole("button", { name: en["compose.writeEmail"] }),
    ).toBeNull();
  });
});
