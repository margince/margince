// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { RecordDetails } from "../../design-system/recorddetails";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { useContactAboutSections } from "./contactdetails";
import { Employers } from "./contactemployers";

type Contact360 = components["schemas"]["Contact360"];
type ProfileField = components["schemas"]["ContactProfileField"];

const CONTACT_ID = "4d7c1a52-93e1-4f0b-8a51-2f6c0d9b7e10";

const signatureCompany: ProfileField = {
  field: "company_name",
  value: "Nordlicht Logistik GmbH",
  evidence_snippet: "Dana Buyer\nHead of Fleet\nNordlicht Logistik GmbH",
  source: "capture_enrich",
  source_ref: "activity:8a0f6c2e-51d4-4b8e-9f3a-6d2e1c7b4a90",
  captured_by: "agent:enrich",
  captured_at: "2026-08-10T09:12:00Z",
  claim_key: "profile_field:company_name",
};

function contactView(
  overrides: Partial<Contact360> = {},
  writable = true,
): Contact360 {
  return {
    as_of: "2026-08-18T09:00:00Z",
    sections_omitted: [],
    contact: {
      id: CONTACT_ID,
      full_name: "Dana Buyer",
      source: "capture_enrich",
      captured_by: "agent:enrich",
      created_at: "2026-08-10T09:12:00Z",
      updated_at: "2026-08-10T09:12:00Z",
      writable,
    },
    employments: { data: [], page: { has_more: false } },
    profile_fields: [signatureCompany],
    ...overrides,
  };
}

const ALL_GRANTS: GrantSpec = {
  contact: ["read", "update"],
  relationship: ["read", "create", "update", "delete"],
  company: ["read", "create"],
};

type Sent = { method: string; path: string; body: unknown; key: string | null };

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The employer write answers with `employer`; every other write succeeds.
function stubApi({
  grants = ALL_GRANTS,
  employer = () =>
    json(
      {
        contact: { id: CONTACT_ID, full_name: "Dana Buyer" },
        company_id: "b91e0c37-2a64-4c1f-9d85-7e3f0a6b2c18",
        company_created: true,
        company_domain: "nordlicht-logistik.de",
        relationship_id: "e5a2d9c4-7b13-4f60-8c2e-1a9f3d6b0e74",
      },
      201,
    ),
}: {
  grants?: GrantSpec;
  employer?: () => Response;
} = {}) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      if (request.method !== "GET") {
        const raw = await request.clone().text();
        sent.push({
          method: request.method,
          path: url.pathname.replace(/^\/v1/, ""),
          body: raw === "" ? null : JSON.parse(raw),
          key: request.headers.get("Idempotency-Key"),
        });
      }
      if (url.pathname.endsWith("/me"))
        return json(meFixture({ allow: grants }));
      if (url.pathname.endsWith("/employer")) return employer();
      if (
        url.pathname.endsWith("/companies/7c4b2e90-1d3a-4f58-b6e2-9a0c5d8f1e23")
      ) {
        return json({
          id: "7c4b2e90-1d3a-4f58-b6e2-9a0c5d8f1e23",
          display_name: "Nordlicht Logistik AG",
        });
      }
      if (url.pathname.endsWith("/companies")) {
        return json({
          data: [
            {
              id: "7c4b2e90-1d3a-4f58-b6e2-9a0c5d8f1e23",
              display_name: "Nordlicht Logistik AG",
            },
          ],
          page: { has_more: false },
        });
      }
      if (request.method === "POST") return json({}, 201);
      return json({ data: [], page: { has_more: false } });
    }),
  );
  return sent;
}

function AboutRail({ view }: Readonly<{ view: Contact360 }>) {
  return <RecordDetails sections={useContactAboutSections(view)} />;
}

const COMPANIES_LIST = ["companies", {}];

function mount(view: Contact360, place: "card" | "rail" = "card") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(COMPANIES_LIST, { data: [], page: { has_more: false } });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {place === "card" ? (
            <Employers view={view} />
          ) : (
            <AboutRail view={view} />
          )}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

const PROPOSAL = "From their email signature: Nordlicht Logistik GmbH";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("The signature’s company on the Company card", () => {
  it("renders a pending company fact with its verbs", async () => {
    stubApi();
    mount(contactView());

    expect(await screen.findByRole("button", { name: "Link" })).toBeEnabled();
    expect(screen.getByText(PROPOSAL)).toBeInTheDocument();
    expect(
      screen.getByText("Link or create this company?"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Create and link" }),
    ).toBeEnabled();
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeEnabled();
  });

  it("creates and links the company, then confirms the fact", async () => {
    const sent = stubApi();
    const client = mount(contactView());
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: "Create and link" }),
    );

    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[0]).toMatchObject({
      method: "POST",
      path: `/contacts/${CONTACT_ID}/employer`,
      body: { company_name: "Nordlicht Logistik GmbH" },
    });
    expect(sent[0]?.key).toMatch(/^[0-9a-f-]{36}$/);
    expect(sent[1]).toMatchObject({
      path: "/ai/feedback",
      body: {
        subject_type: "contact",
        subject_id: CONTACT_ID,
        claim_kind: "profile_field",
        claim_path: "profile_field:company_name",
        verdict: "confirmed",
        value_shown: "Nordlicht Logistik GmbH",
        value_captured_at: "2026-08-10T09:12:00Z",
      },
    });
    expect(
      await screen.findByText(
        "Nordlicht Logistik GmbH created with nordlicht-logistik.de and linked",
      ),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(client.getQueryState(COMPANIES_LIST)?.isInvalidated).toBe(true),
    );
  });

  it("names the company that holds the domain and records nothing", async () => {
    const sent = stubApi({
      employer: () =>
        json(
          {
            type: "about:blank",
            title: "Conflict",
            status: 409,
            code: "duplicate_domain",
            details: { existing_id: "7c4b2e90-1d3a-4f58-b6e2-9a0c5d8f1e23" },
          },
          409,
        ),
    });
    mount(contactView());
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: "Create and link" }),
    );

    expect(
      await screen.findByText(
        "Nordlicht Logistik AG already has this contact’s work email domain, so no company was created.",
      ),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Link that company" }));
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText("Nordlicht Logistik AG"),
    ).toBeInTheDocument();
    expect(sent.map((write) => write.path)).toEqual([
      `/contacts/${CONTACT_ID}/employer`,
    ]);
  });

  it("links through the picker, searching the fact, then confirms it", async () => {
    const sent = stubApi();
    mount(contactView());
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Link" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("searchbox")).toHaveValue(
      "Nordlicht Logistik GmbH",
    );
    await user.click(
      await within(dialog).findByRole("button", {
        name: "Nordlicht Logistik AG",
      }),
    );
    await user.click(within(dialog).getByRole("button", { name: "Create" }));

    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[0]).toMatchObject({
      path: "/relationships",
      body: {
        kind: "employment",
        contact_id: CONTACT_ID,
        company_id: "7c4b2e90-1d3a-4f58-b6e2-9a0c5d8f1e23",
        is_current_primary: true,
      },
    });
    expect(sent[1]).toMatchObject({
      path: "/ai/feedback",
      body: { verdict: "confirmed", claim_path: "profile_field:company_name" },
    });
  });

  it("dismisses the fact so it is not offered again", async () => {
    const sent = stubApi();
    mount(contactView());
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "Dismiss" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toMatchObject({
      path: "/ai/feedback",
      body: { verdict: "suppressed", value_shown: "Nordlicht Logistik GmbH" },
    });
  });

  it("offers nothing to a contact who already works somewhere", async () => {
    stubApi();
    mount(
      contactView({
        employments: {
          data: [
            {
              relationship_id: "e5a2d9c4-7b13-4f60-8c2e-1a9f3d6b0e74",
              company_id: "b91e0c37-2a64-4c1f-9d85-7e3f0a6b2c18",
              company_name: "Brandt Automotive GmbH",
              is_current_primary: true,
              ended_at: null,
            },
          ],
          page: { has_more: false },
        },
      }),
    );

    expect(
      await screen.findByText("Brandt Automotive GmbH"),
    ).toBeInTheDocument();
    expect(screen.queryByText(PROPOSAL)).toBeNull();
  });

  it("offers nothing for a fact somebody already ruled on", async () => {
    stubApi();
    mount(
      contactView({
        profile_fields: [{ ...signatureCompany, verdict: "confirmed" }],
      }),
    );

    expect(
      await screen.findByText("No employment on record."),
    ).toBeInTheDocument();
    expect(screen.queryByText(PROPOSAL)).toBeNull();
  });

  it("refuses both links to a role that may not link companies", async () => {
    stubApi({
      grants: { contact: ["read", "update"], company: ["read", "create"] },
    });
    mount(contactView());

    const reason = await screen.findByText(
      "Your role cannot link companies to contacts. Ask an admin.",
    );
    expect(screen.getByRole("button", { name: "Link" })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Create and link" }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: "Link" })).toHaveAttribute(
      "aria-describedby",
      expect.stringContaining(reason.id),
    );
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeEnabled();
  });

  it("refuses only the create to a role that may not create companies", async () => {
    stubApi({
      grants: {
        contact: ["read", "update"],
        relationship: ["read", "create"],
        company: ["read"],
      },
    });
    mount(contactView());

    expect(
      await screen.findByText(
        "Your role cannot create companies. Ask an admin.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Create and link" }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: "Link" })).toBeEnabled();
  });

  it("shows the fact without verbs to a reader who may not edit the contact", async () => {
    stubApi();
    mount(contactView({}, false));

    expect(await screen.findByText(PROPOSAL)).toBeInTheDocument();
    expect(screen.queryByText("Link or create this company?")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Create and link" }),
    ).toBeNull();
  });
});

describe("The signature’s company in the rail’s About section", () => {
  it("shows the proposal and its verbs on the employer row", async () => {
    stubApi();
    mount(contactView(), "rail");

    expect(
      await screen.findByRole("button", { name: "Create and link" }),
    ).toBeEnabled();
    expect(screen.getByText("Employer")).toBeVisible();
    expect(screen.getByText(PROPOSAL)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Link" })).toBeEnabled();
  });

  it("creates and links through the same requests as the Company card", async () => {
    const sent = stubApi();
    mount(contactView(), "rail");
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: "Create and link" }),
    );

    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent.map((write) => [write.path, write.body])).toEqual([
      [
        `/contacts/${CONTACT_ID}/employer`,
        { company_name: "Nordlicht Logistik GmbH" },
      ],
      ["/ai/feedback", expect.objectContaining({ verdict: "confirmed" })],
    ]);
  });

  it("folds the empty employer row when no company was read", async () => {
    stubApi();
    mount(contactView({ profile_fields: [] }), "rail");

    expect(await screen.findByText(/^Show \d+ empty fields?$/)).toBeVisible();
    expect(screen.queryByText("Employer")).toBeNull();
    expect(screen.queryByText(PROPOSAL)).toBeNull();
  });

  it("folds it once somebody ruled on the company", async () => {
    stubApi();
    mount(
      contactView({
        profile_fields: [{ ...signatureCompany, verdict: "suppressed" }],
      }),
      "rail",
    );

    expect(await screen.findByText(/^Show \d+ empty fields?$/)).toBeVisible();
    expect(screen.queryByText(PROPOSAL)).toBeNull();
  });
});
