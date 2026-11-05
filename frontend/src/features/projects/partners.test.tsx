/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toggleOptions } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { PartnersScreen, PartnerTab } from "./partners";

// The Partner tab on a company and the #/partners list. The tab reads a 404
// from GET /companies/{id}/partner as "not a partner yet" and shows the set-up
// form, never an error. The list reads GET /partners with kind, stage and cert
// filters, and a row click opens that company.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// The seat a spec runs as when it is not about the grants: one that may set
// the margin tier.
const PARTNER_WRITER = meFixture({
  allow: { company: ["read", "update"], partner: ["read", "update"] },
});

function stubFetch(
  responder: (
    url: string,
    method: string,
    request: Request,
  ) => Promise<Response>,
  me = PARTNER_WRITER,
) {
  const urls: string[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    const path = new URL(request.url).pathname;
    if (path.endsWith("/me")) {
      return jsonResponse(me);
    }
    if (path.endsWith("/partner-stages")) {
      return jsonResponse({ data: STAGES });
    }
    urls.push(request.url);
    return responder(request.url, request.method, request);
  });
  vi.stubGlobal("fetch", fetchMock);
  return { fetchMock, urls };
}

function stage(key: string, label: string, sort: number, active = true) {
  return {
    id: `s-${key}`,
    key,
    label,
    label_key: null,
    meaning: "candidate",
    sort_order: sort,
    active,
    system: true,
    partner_count: 0,
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

// An installation's own ladder: renamed, with a stage retired.
const STAGES = [
  stage("scouting", "Scouting", 5),
  stage("research", "Research", 10, false),
  stage("contract_signed", "Signed up", 70),
  stage("active", "Live", 100),
];

const partner = {
  company_id: "o-1",
  partner_kinds: ["hosting", "referral"],
  partner_role: "hosting",
  cert_status: "certified",
  margin_tier: "tier2_20",
  relationship_stage: "active",
  stage_label: "Live",
  stage_meaning: "active",
  activated_at: "2026-05-04T10:00:00Z",
  next_step: "Renew certification",
  next_step_due_at: "2026-08-01",
  served_segments: ["mid-market"],
  version: 3,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

describe("PartnerTab — not yet a partner", () => {
  it("shows the setup form and PUTs the chosen kinds with no If-Match", async () => {
    const user = userEvent.setup();
    let putBody: unknown = null;
    let putHeader: string | null = null;
    stubFetch(async (url, method, request) => {
      if (url.includes("/companies/o-1/partner") && method === "GET") {
        return jsonResponse({ title: "Not found", detail: "no partner" }, 404);
      }
      if (url.includes("/companies/o-1/partner") && method === "PUT") {
        putHeader = request.headers.get("If-Match");
        putBody = JSON.parse(await request.text());
        return jsonResponse({ ...partner, cert_status: "applied" });
      }
      throw new Error(`unexpected request ${method} ${url}`);
    });

    render(<PartnerTab companyId="o-1" />);

    await waitFor(() =>
      expect(screen.getByText("Not a partner yet")).toBeTruthy(),
    );
    const create = screen.getByRole("button", { name: "Create" });
    expect(create).toBeDisabled();
    await toggleOptions(user, screen.getByLabelText("Partner kinds *"), [
      "Referral",
      "Consulting",
    ]);
    await user.click(create);

    await waitFor(() => expect(putBody).toBeTruthy());
    expect(putBody).toMatchObject({
      partner_kinds: ["consulting", "referral"],
    });
    expect(putBody).not.toHaveProperty("relationship_stage");
    expect(putHeader).toBeNull();
  });

  it("says what the margin tier pays and gives an example segment", async () => {
    stubFetch(async (url, method) => {
      if (url.includes("/companies/o-1/partner") && method === "GET") {
        return jsonResponse({ title: "Not found", detail: "no partner" }, 404);
      }
      throw new Error(`unexpected request ${method} ${url}`);
    });

    render(<PartnerTab companyId="o-1" />);

    const tier = await screen.findByLabelText("Margin tier");
    expect(tier).toHaveAccessibleDescription(
      /share the partner earns on each won deal they sourced/,
    );
    expect(
      screen.getByLabelText("Served segments"),
    ).toHaveAccessibleDescription(/for example mid-size manufacturers/);
  });
});

describe("PartnerTab — a company editor without the partner grant", () => {
  const COMPANY_EDITOR = meFixture({
    allow: { company: ["read", "update"], partner: ["read"] },
  });

  async function setUpAs(me: ReturnType<typeof meFixture>) {
    const user = userEvent.setup();
    let putBody: Record<string, unknown> | null = null;
    stubFetch(async (url, method, request) => {
      if (url.includes("/companies/o-1/partner") && method === "GET") {
        return jsonResponse({ title: "Not found", detail: "no partner" }, 404);
      }
      if (url.includes("/companies/o-1/partner") && method === "PUT") {
        putBody = JSON.parse(await request.text());
        return jsonResponse({ ...partner, margin_tier: null });
      }
      throw new Error(`unexpected request ${method} ${url}`);
    }, me);
    render(<PartnerTab companyId="o-1" />);
    const tier = await screen.findByLabelText("Margin tier");
    return {
      tier,
      create: async () => {
        await toggleOptions(user, screen.getByLabelText("Partner kinds *"), [
          "Hosting",
        ]);
        await user.click(screen.getByRole("button", { name: "Create" }));
        await waitFor(() => expect(putBody).toBeTruthy());
        return putBody ?? {};
      },
    };
  }

  it("sets up the program with the tier disabled and left out of the request", async () => {
    const { tier, create } = await setUpAs(COMPANY_EDITOR);

    await waitFor(() =>
      expect(tier).toHaveAccessibleDescription(
        "Your role cannot set the margin tier. Ask an admin.",
      ),
    );
    expect(tier).toBeDisabled();
    const body = await create();
    expect(body).toMatchObject({ partner_kinds: ["hosting"] });
    expect(body).not.toHaveProperty("margin_tier");
  });

  it("lets the partner grant set the tier and send it", async () => {
    const { tier, create } = await setUpAs(PARTNER_WRITER);

    await waitFor(() => expect(tier).toBeEnabled());
    expect(await create()).toHaveProperty("margin_tier", null);
  });
});

describe("PartnerTab — a seat without the company grant", () => {
  const PARTNER_ONLY = meFixture({
    allow: { company: ["read"], partner: ["read", "update"] },
  });
  const REFUSAL =
    "Your role cannot change this company’s partner program. Ask an admin.";

  it("offers no set-up form, and says why", async () => {
    stubFetch(async (url, method) => {
      if (url.includes("/companies/o-1/partner") && method === "GET") {
        return jsonResponse({ title: "Not found", detail: "no partner" }, 404);
      }
      throw new Error(`unexpected request ${method} ${url}`);
    }, PARTNER_ONLY);
    render(<PartnerTab companyId="o-1" />);

    expect(await screen.findByText(REFUSAL)).toBeTruthy();
    expect(screen.queryByLabelText("Margin tier")).toBeNull();
    expect(screen.queryByRole("button", { name: "Create" })).toBeNull();
  });

  it("refuses the edit, and says why", async () => {
    stubFetch(async (url, method) => {
      if (url.includes("/companies/o-1/partner") && method === "GET") {
        return jsonResponse(partner);
      }
      throw new Error(`unexpected request ${method} ${url}`);
    }, PARTNER_ONLY);
    render(<PartnerTab companyId="o-1" />);

    const edit = await screen.findByTestId("edit-partner");
    await waitFor(() => expect(edit).toBeDisabled());
    expect(edit).toHaveAccessibleDescription(REFUSAL);
  });
});

describe("PartnerTab — existing partner", () => {
  it("shows its fields and edits with If-Match", async () => {
    let putHeader: string | null = null;
    let putBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (url.includes("/companies/o-1/partner") && method === "GET") {
        return jsonResponse(partner);
      }
      if (url.includes("/companies/o-1/partner") && method === "PUT") {
        putHeader = request.headers.get("If-Match");
        putBody = JSON.parse(await request.text());
        return jsonResponse({ ...partner, next_step: "Book QBR" });
      }
      throw new Error(`unexpected request ${method} ${url}`);
    });

    render(<PartnerTab companyId="o-1" />);

    await waitFor(() =>
      expect(screen.getByText("Renew certification")).toBeTruthy(),
    );
    expect(screen.getByText("Hosting, Referral")).toBeTruthy();
    expect(screen.getByText("Live")).toBeTruthy();
    await userEvent.click(screen.getByTestId("edit-partner"));

    const nextStep = await screen.findByLabelText("Next step");
    await userEvent.clear(nextStep);
    await userEvent.type(nextStep, "Book QBR");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(putBody).toBeTruthy());
    expect(putHeader).toBe("3");
    expect(putBody).toMatchObject({
      next_step: "Book QBR",
      partner_kinds: ["hosting", "referral"],
      relationship_stage: "active",
    });
  });

  it("offers the active stages in ladder order and the retired one the partner stands on", async () => {
    const user = userEvent.setup();
    stubFetch(async (url, method) => {
      if (url.includes("/companies/o-1/partner") && method === "GET") {
        return jsonResponse({
          ...partner,
          relationship_stage: "research",
          stage_label: "Research",
        });
      }
      throw new Error(`unexpected request ${method} ${url}`);
    });
    render(<PartnerTab companyId="o-1" />);

    await user.click(await screen.findByTestId("edit-partner"));
    await user.click(await screen.findByLabelText("Relationship stage"));
    const listbox = await screen.findByRole("listbox");
    expect(
      within(listbox)
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["Scouting", "Research", "Signed up", "Live"]);
  });
});

describe("PartnerTab — the margin tier", () => {
  function stubPartner(body: Record<string, unknown>) {
    stubFetch(async (url, method) => {
      if (url.includes("/companies/o-1/partner") && method === "GET") {
        return jsonResponse(body);
      }
      throw new Error(`unexpected request ${method} ${url}`);
    });
  }

  it("names the tier a reader may see", async () => {
    stubPartner(partner);

    render(<PartnerTab companyId="o-1" />);

    expect(await screen.findByText("Active collaboration (20%)")).toBeTruthy();
    expect(screen.queryByLabelText("Masked value")).toBeNull();
  });

  it("reads a withheld tier as withheld, never as a partner with no tier", async () => {
    stubPartner({
      ...partner,
      margin_tier: null,
      masked_fields: ["margin_tier"],
    });

    render(<PartnerTab companyId="o-1" />);

    expect(await screen.findByText("Margin tier")).toBeTruthy();
    expect(screen.getByLabelText("Masked value")).toBeTruthy();
  });

  it("omits the row for a partner who agreed no tier", async () => {
    stubPartner({ ...partner, margin_tier: null });

    render(<PartnerTab companyId="o-1" />);

    await screen.findByText("Renew certification");
    expect(screen.queryByText("Margin tier")).toBeNull();
    expect(screen.queryByLabelText("Masked value")).toBeNull();
  });
});

// The columns picker offers a checkbox per column under the header's text, so
// a plain name match is ambiguous. Each click is scoped to the filter menu. The
// menu is named "Filter" while it lists attributes, then by the attribute.
async function openFilterMenu(): Promise<HTMLElement> {
  await userEvent.click(screen.getByRole("button", { name: "Filter" }));
  return screen.getByRole("group", { name: "Filter" });
}

async function pickFilter(attribute: string, value: string) {
  await userEvent.click(screen.getByRole("button", { name: "Filter" }));
  const step = (name: string) => screen.getByRole("group", { name });
  await userEvent.click(
    within(step("Filter")).getByRole("button", { name: attribute }),
  );
  await userEvent.click(
    within(step(attribute)).getByRole("radio", { name: value }),
  );
}

describe("PartnersScreen", () => {
  it("lists partners and sends the kind filter", async () => {
    const { urls } = stubFetch(async () =>
      jsonResponse({
        data: [partner],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<PartnersScreen />);

    await waitFor(() => expect(screen.getByText("o-1")).toBeTruthy());
    expect(screen.getByText("Live")).toBeTruthy();
    expect(screen.getByText("Referral")).toBeTruthy();

    await pickFilter("Partner kinds", "Referral");

    await waitFor(() =>
      expect(urls.some((url) => url.includes("partner_kind=referral"))).toBe(
        true,
      ),
    );
  });

  it("renders no search box (GET /partners has no q param) but keeps the kind/stage/cert filters", async () => {
    stubFetch(async () =>
      jsonResponse({
        data: [partner],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<PartnersScreen />);

    await waitFor(() => expect(screen.getByText("o-1")).toBeTruthy());

    expect(screen.queryByPlaceholderText("Search")).toBeNull();
    expect(screen.queryByLabelText("Show archived")).toBeNull();
    const menu = await openFilterMenu();
    for (const name of [
      "Partner kinds",
      "Stage meaning",
      "Relationship stage",
    ]) {
      expect(within(menu).getByRole("button", { name })).toBeTruthy();
    }
    expect(
      within(menu).getByRole("button", { name: "Certification status" }),
    ).toBeTruthy();
  });

  it("navigates to the company's 360 on row click", async () => {
    stubFetch(async () =>
      jsonResponse({
        data: [partner],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<PartnersScreen />);

    await waitFor(() => expect(screen.getByText("o-1")).toBeTruthy());
    await userEvent.click(screen.getByText("o-1"));

    expect(window.location.hash).toBe("#/companies/o-1");
  });
});

describe("PartnerTab — activation", () => {
  function stubPartner(body: Record<string, unknown>) {
    stubFetch(async (url, method) => {
      if (url.includes("/companies/o-1/partner") && method === "GET") {
        return jsonResponse(body);
      }
      return jsonResponse({ id: "l-7", full_name: "First Referral" });
    });
  }

  it("says a partner active before the stage list has no recorded date", async () => {
    stubPartner({
      ...partner,
      activated_at: null,
      activation_basis: "recorded_before_ladder",
    });
    render(<PartnerTab companyId="o-1" />);
    expect(
      await screen.findByText(/Active before the stage list was introduced/),
    ).toBeTruthy();
    expect(screen.queryByText(/Activated/)).toBeNull();
  });

  it("names the lead that activated the partner", async () => {
    stubPartner({
      ...partner,
      activation_basis: "lead_accepted",
      activation_lead_id: "l-7",
    });
    render(<PartnerTab companyId="o-1" />);
    expect(await screen.findByText(/through this lead/)).toBeTruthy();
  });
});
