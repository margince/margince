/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { companyNamesAnswer } from "../../testing/companynames.testkit";
import { LeadScreen } from "./leads";
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

async function showEmptyFields() {
  await userEvent.click(
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
      screen.queryByRole("button", { name: "Change Industry" }),
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

    await userEvent.click(
      await screen.findByRole("button", { name: "Change Industry" }),
    );
    await userEvent.click(
      await screen.findByRole("option", { name: "Manufacturing" }),
    );
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
      await screen.findByRole("button", { name: "Change Industry" }),
    ).toHaveTextContent("Software");
    expect(screen.queryByRole("region", { name: /^From / })).toBeNull();
  });

  it("lets a rep type an industry on an unlinked lead", async () => {
    const patches = serve(lead);
    render(<LeadScreen id="l-1" />);
    await showEmptyFields();
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Industry" }),
    );
    await userEvent.click(
      await screen.findByRole("option", { name: "Logistics and Transport" }),
    );
    await waitFor(() =>
      expect(patches).toEqual([
        expect.objectContaining({ industry: "logistics_transport" }),
      ]),
    );
    expect(screen.queryByRole("region", { name: /^From / })).toBeNull();
  });
});
