/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FORM_DIALOG_MAX_FIELDS } from "../../design-system/modal";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { companyNamesAnswer } from "../../testing/companynames.testkit";
import { listsMe } from "../../testing/lists.fixtures";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub } from "../../testing/story-utils";
import { LeadDetails } from "./leaddetails";
import { LeadScreen } from "./leads";
import { useLeadPatch } from "./leads.patch";
import {
  detailValue,
  editSection,
  jsonResponse,
  LEAD_GRANTS,
  lead,
  render,
  stubFetch,
} from "./leads.testkit";

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

function labels(dialog: HTMLElement): string[] {
  return (
    [...dialog.querySelectorAll("label, legend")]
      // A required field's label ends in its marker, which is not its name.
      .map((label) => label.textContent?.replace(/\s*\*$/, "").trim() ?? "")
      .filter(Boolean)
  );
}

async function chooseIndustry(user: UserEvent, industry: string) {
  const about = await editSection(user, "About");
  await pickOption(
    user,
    within(about).getByRole("combobox", { name: "Industry" }),
    industry,
  );
  await user.click(within(about).getByRole("button", { name: "Save" }));
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
    expect(
      within(group).getByText(en["industry.logisticsTransport"]),
    ).toBeTruthy();
    expect(within(group).getByText("Germany")).toBeTruthy();
    expect(within(group).getByText("51–200 employees")).toBeTruthy();
    expect(
      within(group).getByText(en["lead.details.companyDecides"]),
    ).toBeTruthy();
    const about = await editSection(userEvent.setup(), "About");
    expect(
      within(about).queryByRole("combobox", { name: "Industry" }),
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

    await chooseIndustry(userEvent.setup(), "Manufacturing");
    await waitFor(() =>
      expect(patches).toEqual([
        expect.objectContaining({ industry: "manufacturing" }),
      ]),
    );
  });

  it("shows nothing from the company when the server sends no facts", async () => {
    serve({ ...lead, company_id: "c-7", industry: "software" });
    render(<LeadScreen id="l-1" />);
    expect(await detailValue("Industry")).toHaveTextContent("Software");
    expect(screen.queryByRole("region", { name: /^From / })).toBeNull();
  });

  it("lets a rep type an industry on an unlinked lead", async () => {
    const patches = serve(lead);
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    await chooseIndustry(user, "Logistics and Transport");
    await waitFor(() =>
      expect(patches).toEqual([
        expect.objectContaining({ industry: "logistics_transport" }),
      ]),
    );
    expect(screen.queryByRole("region", { name: /^From / })).toBeNull();
  });
});

describe("a lead's phone", () => {
  it("dials the number from its row", async () => {
    serve({ ...lead, phone: "+49301234567" });
    render(<LeadScreen id="l-1" />);
    const number = await screen.findByRole("link", { name: "+49301234567" });
    expect(number).toHaveAttribute("href", "tel:+49301234567");
    // As on a contact: a block link would push the chat action to a line of its own.
    expect(number).not.toHaveClass("lead-row-link");
  });
});

describe("the Tags row", () => {
  function serveTags(tags: Response, me: unknown = undefined) {
    stubFetch(async (url) => {
      if (url.includes("/records/lead/l-1/tags")) return tags.clone();
      if (me && url.endsWith("/v1/me")) return jsonResponse(me);
      return jsonResponse(lead);
    });
  }
  const reader = {
    user: { id: "u-2", display_name: "Reader" },
    roles: ["rep"],
    teams: [],
    authorization: meFixture({ allow: { lead: ["read"] } }).authorization,
  };

  it("says the words are withheld, to a reader who may only look too", async () => {
    serveTags(jsonResponse({ data: [], withheld: true }), reader);
    render(<LeadScreen id="l-1" />);
    const registration = await screen.findByRole("region", {
      name: en["record.registration"],
    });
    expect(
      await within(registration).findByText(en["tags.withheld"]),
    ).toBeVisible();
  });

  it("folds a lead that carries no tags as an empty field", async () => {
    serveTags(jsonResponse({ data: [] }));
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    const registration = await screen.findByRole("region", {
      name: en["record.registration"],
    });
    await waitFor(() =>
      expect(
        within(registration).queryByText(en["tags.panelTitle"]),
      ).toBeNull(),
    );
    await showEmptyFields(user);
    expect(await detailValue(en["tags.panelTitle"])).toHaveTextContent(
      en["field.unset"],
    );
  });

  it("claims nothing about the tags when their read fails", async () => {
    serveTags(jsonResponse({ detail: "down" }, 500));
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    await showEmptyFields(user);
    await screen.findByText(en["lead.originDetail"], { selector: "dt" });
    expect(
      screen.queryByText(en["tags.panelTitle"], { selector: "dt" }),
    ).toBeNull();
  });
});

describe("the Company routing key row", () => {
  it("says what the key is for", async () => {
    serve(lead);
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    await showEmptyFields(user);
    expect(
      await screen.findByRole("button", {
        name: en["record.companyRoutingKeyWhy"],
      }),
    ).toBeInTheDocument();
  });
});

describe("the About company row", () => {
  it("reads a linked company's name in the dialog as the row reads it, without company read", async () => {
    stubFetch(async (url) => {
      const named = companyNamesAnswer(url, { "c-7": "Nordwind Holding SE" });
      return named ?? jsonResponse({ ...linked, company_facts: undefined });
    });
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    const row = await screen.findByRole("region", { name: "About" });
    await waitFor(() =>
      expect(
        within(row).getByText("Company").nextElementSibling,
      ).toHaveTextContent("Nordwind Holding SE"),
    );
    const about = await editSection(user, "About");
    expect(
      within(about).getByRole("group", { name: "Company" }),
    ).toHaveTextContent("Nordwind Holding SE");
  });

  // The name shown is the company's own, so an editor here would change a name
  // the row does not show.
  it("shows a linked lead's company name read-only, saying where it changes", async () => {
    // A reader without company read gets no company name from a reference.
    stubFetch(async (url) => {
      const named = companyNamesAnswer(url, { "c-7": "Nordwind Holding SE" });
      if (named) return named;
      if (url.endsWith("/v1/me")) {
        return jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
          authorization: meFixture({
            allow: { ...LEAD_GRANTS, company: ["read"] },
          }).authorization,
        });
      }
      return jsonResponse({ ...linked, company_name: "Nordwind Logistik" });
    });
    render(<LeadScreen id="l-1" />);
    const about = await screen.findByRole("region", { name: "About" });
    await waitFor(() =>
      expect(
        within(about).getByText("Company").nextElementSibling,
      ).toHaveTextContent("Nordwind Holding SE"),
    );
    expect(within(about).queryByText("Nordwind Logistik")).toBeNull();
    const dialog = await editSection(userEvent.setup(), "About");
    expect(
      within(dialog).queryByRole("textbox", { name: "Company" }),
    ).toBeNull();
    const company = within(dialog).getByRole("group", { name: "Company" });
    expect(company).toHaveTextContent("Nordwind Holding SE");
    expect(company).toHaveAccessibleDescription(
      "The name comes from the linked company. Rename it on the company’s page.",
    );
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
    expect(await detailValue("Company")).toHaveTextContent("Not set");
    expect(
      within(screen.getByRole("region", { name: "About" })).queryByText(
        "Nordwind Logistik",
      ),
    ).toBeNull();
  });

  it("edits the typed name while the lead is not linked", async () => {
    const patches = serve({ ...lead, company_name: "Nordwind Logistik" });
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    const about = await editSection(user, "About");
    expect(within(about).getByRole("textbox", { name: "Company" })).toHaveValue(
      "Nordwind Logistik",
    );
    expect(patches).toEqual([]);
  });
});

describe("the details' Edit", () => {
  it("offers no Edit to a reader without the lead's write grant", async () => {
    stubFetch(async (url) =>
      url.endsWith("/v1/me")
        ? jsonResponse({
            user: { id: "u-2", display_name: "Reader" },
            roles: ["rep"],
            teams: [],
            authorization: meFixture({ allow: { lead: ["read"] } })
              .authorization,
          })
        : jsonResponse(lead),
    );
    render(<LeadScreen id="l-1" />);
    expect(await detailValue("Company")).toHaveTextContent("Nordwind Logistik");
    expect(screen.queryByRole("button", { name: /^Edit / })).toBeNull();
  });

  it("opens only About's fields and saves them in one PATCH against the version shown", async () => {
    const sent: { ifMatch: string | null; body: unknown }[] = [];
    stubFetch(async (_url, method, request) => {
      if (method === "PATCH") {
        const body: Record<string, unknown> = JSON.parse(await request.text());
        sent.push({ ifMatch: request.headers.get("If-Match"), body });
        return jsonResponse({ ...lead, ...body, version: 2 });
      }
      return jsonResponse(lead);
    });
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    const about = await editSection(user, "About");
    expect(within(about).queryByRole("combobox", { name: "Owner" })).toBeNull();
    const title = within(about).getByRole("textbox", { name: "Job title" });
    await user.type(title, "Head of fleet");
    await user.click(within(about).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(sent).toEqual([
        {
          ifMatch: expect.stringContaining("1"),
          body: expect.objectContaining({ title: "Head of fleet" }),
        },
      ]),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "About" })).toBeNull(),
    );
  });

  it("saves a nameless lead's title without asking for the name first", async () => {
    const patches = serve({ ...lead, full_name: null });
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    const about = await editSection(user, "About");
    expect(
      within(about).getByRole("textbox", { name: "Full name" }),
    ).toHaveValue("");
    await user.type(
      within(about).getByRole("textbox", { name: "Job title" }),
      "Fleet manager",
    );
    await user.click(within(about).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(patches).toEqual([
        expect.objectContaining({ title: "Fleet manager" }),
      ]),
    );
    expect(patches[0]).not.toHaveProperty("full_name");
  });

  it("opens each section with only its own fields, none past a small form", async () => {
    serve({ ...lead, full_name: null, partner_company_id: "c-9" });
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    const expected: Record<string, string[]> = {
      About: [
        en["create.fullName"],
        en["create.companyName"],
        en["create.contactTitle"],
        en["create.industry"],
        en["lead.project"],
        en["lead.product"],
      ],
      [en["record.reach.label"]]: [
        en["create.salutation"],
        en["create.email"],
        en["create.phone"],
        en["create.linkedinUrl"],
      ],
      [en["record.registration"]]: [
        en["lead.origin"],
        en["lead.originDetail"],
        en["lead.partner"],
        en["record.companyRoutingKey"],
        en["tags.panelTitle"],
      ],
    };
    for (const [section, fields] of Object.entries(expected)) {
      const dialog = await editSection(user, section);
      expect(labels(dialog)).toEqual(fields);
      expect(labels(dialog).length).toBeLessThanOrEqual(FORM_DIALOG_MAX_FIELDS);
      await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
      await waitFor(() =>
        expect(screen.queryByRole("dialog", { name: section })).toBeNull(),
      );
    }
  });

  it("leaves the dialog for the project search, and gives focus back to Edit", async () => {
    serve(lead);
    const user = userEvent.setup();
    render(<LeadScreen id="l-1" />);
    const about = await editSection(user, "About");
    await user.click(
      within(about).getByRole("button", { name: "Change Project" }),
    );
    const search = await screen.findByRole("dialog", {
      name: en["lead.rail.project.attach"],
    });
    expect(screen.queryByRole("dialog", { name: "About" })).toBeNull();
    expect(
      within(search).getByRole("combobox", {
        name: en["projectLinks.searchLabel"],
      }),
    ).toBeVisible();
    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Edit About" })).toHaveFocus(),
    );
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
