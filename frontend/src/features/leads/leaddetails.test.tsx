/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { changeChoice } from "../../testing/choicedialog.testkit";
import { companyNamesAnswer } from "../../testing/companynames.testkit";
import { listsMe } from "../../testing/lists.fixtures";
import { installFetchStub } from "../../testing/story-utils";
import { LeadDetails } from "./leaddetails";
import { LeadScreen } from "./leads";
import { useLeadPatch } from "./leads.patch";
import { jsonResponse, lead, render, stubFetch } from "./leads.testkit";

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

const linked = {
  ...lead,
  company_id: "c-7",
  company_facts: {
    industry: "logistics_transport",
    country: "DE",
    size_band: "51-200",
  },
} as const;

// Serves the lead as given and the company its link names. Every PATCH body is
// kept, so a test can read what an edit sent.
function serve(served: Record<string, unknown>) {
  const patches: Record<string, unknown>[] = [];
  stubFetch(async (url, method, request) => {
    const named = companyNamesAnswer(url, { "c-7": "Nordwind Holding SE" });
    if (named) {
      return named;
    }
    if (method === "PATCH") {
      const body: Record<string, unknown> = JSON.parse(await request.text());
      patches.push(body);
      return jsonResponse({ ...served, ...body, version: 2 });
    }
    return jsonResponse(served);
  });
  return patches;
}

async function showEmptyFields(user: UserEvent) {
  await user.click(
    await screen.findByRole("button", { name: /^Show \d+ empty fields?$/ }),
  );
}

describe("a lead's company facts", () => {
  it("shows the linked company's industry, country and size, named for that company", async () => {
    serve(linked);
    render(<LeadScreen id="l-1" />);
    const group = await screen.findByRole("region", {
      name: "From Nordwind Holding SE",
    });
    expect(within(group).getByText("Logistics and Transport")).toBeTruthy();
    expect(within(group).getByText("Germany")).toBeTruthy();
    expect(within(group).getByText("51–200 employees")).toBeTruthy();
    expect(
      within(group).getByText(
        "These come from the company record. Change them on the company.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /^Change Industry/ }),
    ).toBeNull();
  });

  it("shows only the facts the company holds, and the lead's own industry while it holds none", async () => {
    const patches = serve({
      ...linked,
      industry: "software",
      company_facts: { industry: null, country: null, size_band: "1-10" },
    });
    render(<LeadScreen id="l-1" />);
    const group = await screen.findByRole("region", {
      name: "From Nordwind Holding SE",
    });
    expect(within(group).getByText("1–10 employees")).toBeTruthy();
    expect(within(group).queryByText("Country")).toBeNull();
    expect(within(group).queryByText("Industry")).toBeNull();

    await changeChoice(userEvent.setup(), "Industry", "Manufacturing");
    await waitFor(() =>
      expect(patches).toEqual([
        expect.objectContaining({ industry: "manufacturing" }),
      ]),
    );
  });

  it("shows nothing from the company when the server sends no facts", async () => {
    serve({ ...lead, company_id: "c-7", industry: "software" });
    render(<LeadScreen id="l-1" />);
    expect(
      await screen.findByRole("button", { name: /^Change Industry/ }),
    ).toHaveTextContent("Software");
    expect(screen.queryByRole("region", { name: /^From / })).toBeNull();
  });

  it("lets a rep type an industry on an unlinked lead", async () => {
    const patches = serve(lead);
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    await showEmptyFields(user);
    await changeChoice(user, "Industry", "Logistics and Transport");
    await waitFor(() =>
      expect(patches).toEqual([
        expect.objectContaining({ industry: "logistics_transport" }),
      ]),
    );
    expect(screen.queryByRole("region", { name: /^From / })).toBeNull();
  });
});

describe("the About company row", () => {
  // The name shown is the company's own, so an editor here would change a name
  // the row does not show.
  it("shows a linked lead's company name read-only, saying where it changes", async () => {
    serve({ ...linked, company_name: "Nordwind Logistik" });
    render(<LeadScreen id="l-1" />);
    const name = await screen.findByText("Nordwind Holding SE", {
      selector: ".inlinetext",
    });
    expect(name).toHaveAttribute(
      "title",
      "The name comes from the linked company. Rename it on the company’s page.",
    );
    expect(screen.queryByText("Nordwind Logistik")).toBeNull();
    expect(
      screen.queryByRole("button", { name: /^Change Company/ }),
    ).toBeNull();
  });

  it("shows the unset text, not the typed name, when the linked company has no name", async () => {
    stubFetch(async (url) => {
      const named = companyNamesAnswer(url, {});
      return (
        named ?? jsonResponse({ ...linked, company_name: "Nordwind Logistik" })
      );
    });
    render(<LeadScreen id="l-1" />);
    await showEmptyFields(userEvent.setup());
    expect(
      await screen.findByText("Not set", { selector: ".inlinetext" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Nordwind Logistik", { selector: ".inlinetext" }),
    ).toBeNull();
  });

  it("edits the typed name while the lead is not linked", async () => {
    const patches = serve({ ...lead, company_name: "Nordwind Logistik" });
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    const change = await screen.findByRole("button", {
      name: /^Change Company/,
    });
    expect(change).toHaveTextContent("Nordwind Logistik");
    await user.click(change);
    expect(await screen.findByDisplayValue("Nordwind Logistik")).toBeVisible();
    expect(patches).toEqual([]);
  });
});

describe("a lead's Lists group", () => {
  function Details() {
    const writer = useLeadPatch(lead, lead.id, () => undefined);
    return (
      <LeadDetails lead={lead} writer={writer} terminalReasonId="refused" />
    );
  }

  function show(onLists: unknown[] = []) {
    installFetchStub({
      "GET /me": listsMe(true, [], {
        allow: { lead: ["read", "update"], list: ["read", "update"] },
      }),
      [`GET /records/lead/${lead.id}/lists`]: () =>
        jsonResponse({ data: onLists }),
    });
    render(<Details />);
  }

  const group = async () =>
    within(
      await screen.findByRole("region", { name: en["lists.record.title"] }),
    );

  it("offers Add to Shortlist and every list while the lead is on none", async () => {
    show();
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
    show([{ id: "s-1", name: "Launch references", entity_type: "lead" }]);
    const lists = await group();
    expect(await lists.findByText("Launch references")).toBeTruthy();
    expect(
      lists.getByRole("button", { name: en["lists.addToShortlist"] }),
    ).toBeTruthy();
  });
});
