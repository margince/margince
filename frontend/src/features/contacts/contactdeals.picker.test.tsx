/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ContactDealsTab } from "./contactdeals";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const view: components["schemas"]["Contact360"] = {
  as_of: "2026-01-10T09:00:00Z",
  sections_omitted: [],
  contact: {
    id: "p1",
    full_name: "Buyer Contact",
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-01-01T09:00:00Z",
    updated_at: "2026-01-01T09:00:00Z",
  },
  deal_roles: { data: [], page: { has_more: false } },
};

const first = { id: "d1", name: "Employer A renewal", company_id: "o1" };
const second = { id: "d2", name: "Employer B renewal", company_id: "o2" };
const partner = {
  id: "old-deal",
  name: "Partner renewal",
  company_id: "o3",
  created_at: "2000-01-01T00:00:00Z",
};
const companies = [
  { id: "o1", display_name: "Employer A" },
  { id: "o2", display_name: "Employer B" },
  { id: "o3", display_name: "Partner company" },
];

type Scenario = {
  employers: string[];
  paged?: boolean;
  companyFailure?: number;
  withheld?: boolean;
  contextFailure?: boolean;
};
const scenarios: Scenario[] = [
  { employers: [] },
  { employers: ["o1", "o2"] },
  { employers: ["o1", "o2"], paged: true },
  { employers: ["o1", "o2"], companyFailure: 403 },
  { employers: ["o1", "o2"], companyFailure: 500 },
  { employers: [], withheld: true },
  { employers: [], contextFailure: true },
];
it.each(scenarios)(
  "opens current-employer deals on focus and searches other open deals: $employers",
  async ({ employers, paged, companyFailure, withheld, contextFailure }) => {
    const employmentQueries: URL[] = [];
    const queries: URL[] = [];
    stubWithSession(
      {},
      {
        relationship: withheld ? ["create"] : ["read", "create"],
        contact: ["read"],
        company: ["read"],
        deal: ["read"],
      },
    );
    const fallback = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(
          input instanceof Request ? input.url : String(input),
          "https://test.local",
        );
        const path = url.pathname.replace(/^\/v1/, "");
        if (path === "/contacts/p1/360") {
          if (contextFailure)
            return jsonResponse(
              {
                title: "Unavailable",
                status: 503,
                code: "service_unavailable",
              },
              503,
            );
          if (withheld)
            return jsonResponse({ ...view, sections_omitted: ["employments"] });
          return jsonResponse({
            ...view,
            employments: {
              data: [
                ...(paged ? employers.slice(0, 1) : employers).map(
                  (company, index) => ({
                    relationship_id: `r${index}`,
                    company_id: company,
                    is_current_primary: index === 0,
                    employment_status: "current",
                  }),
                ),
                {
                  relationship_id: "former",
                  company_id: "former",
                  is_current_primary: false,
                  employment_status: "former",
                },
              ],
              page: {
                has_more: Boolean(paged),
                next_cursor: paged ? "later-employments" : null,
              },
            },
          });
        }
        if (path === "/relationships") {
          employmentQueries.push(url);
          return jsonResponse({
            data: [
              {
                id: "later-role",
                kind: "employment",
                contact_id: "p1",
                company_id: "o2",
                is_current_primary: false,
                employment_status: "current",
              },
            ],
            page: { has_more: false },
          });
        }
        if (path === "/companies") {
          if (companyFailure)
            return jsonResponse(
              {
                title: "Company names unavailable",
                status: companyFailure,
                code:
                  companyFailure === 403
                    ? "permission_denied"
                    : "internal_error",
              },
              companyFailure,
            );
          return jsonResponse({ data: companies, page: { has_more: false } });
        }
        if (path === "/deals") {
          queries.push(url);
          const company = url.searchParams.get("company_id");
          const data =
            company === "o1"
              ? [first]
              : company === "o2"
                ? [second]
                : url.searchParams.get("q")
                  ? [partner, first]
                  : [partner];
          return jsonResponse({
            data:
              url.searchParams.get("status") === "open"
                ? data
                : [
                    ...data,
                    { id: "won", name: "Won renewal" },
                    { id: "lost", name: "Lost renewal" },
                  ],
            page: { has_more: false },
          });
        }
        return fallback(input, init);
      },
    );
    render(
      <StoryProviders>
        <ContactDealsTab view={view} />
      </StoryProviders>,
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Add to a deal" }),
    );
    const dialog = screen.getByRole("dialog");
    const input = within(dialog).getByRole("searchbox");
    await user.click(input);
    if (withheld || contextFailure) {
      await within(dialog).findByText(
        "Employer details are unavailable. Type to search all open deals.",
      );
      expect(queries).toHaveLength(0);
    } else if (employers.length) {
      await within(dialog).findByRole("button", { name: /Employer A renewal/ });
      expect(
        within(dialog).getByRole("button", { name: /Employer B renewal/ }),
      ).toBeTruthy();
      expect(
        within(dialog).queryByRole("button", { name: /Partner renewal/ }),
      ).toBeNull();
      expect(
        new Set(queries.map((url) => url.searchParams.get("company_id"))),
      ).toEqual(new Set(employers));
    } else {
      await within(dialog).findByRole("button", { name: /Partner renewal/ });
      expect(queries.some((url) => !url.searchParams.has("company_id"))).toBe(
        true,
      );
    }
    await user.type(input, "renewal");
    await waitFor(() =>
      expect(
        queries.some(
          (url) =>
            url.searchParams.get("q") === "renewal" &&
            !url.searchParams.has("company_id"),
        ),
      ).toBe(true),
    );
    await within(dialog).findByRole("button", { name: /Partner renewal/ });
    expect(
      queries.every((url) => url.searchParams.get("status") === "open"),
    ).toBe(true);
    expect(queries.every((url) => url.searchParams.get("limit") !== "50")).toBe(
      true,
    );
    if (companyFailure)
      expect(
        within(dialog).getAllByText("Name did not load").length,
      ).toBeGreaterThan(0);
    else expect(within(dialog).getByText("Partner company")).toBeTruthy();
    if (paged) {
      expect(
        employmentQueries.some(
          (url) => url.searchParams.get("cursor") === "later-employments",
        ),
      ).toBe(true);
      expect(
        employmentQueries.every(
          (url) =>
            url.searchParams.get("contact_id") === "p1" &&
            url.searchParams.get("kind") === "employment",
        ),
      ).toBe(true);
    }
    expect(
      within(dialog).queryByRole("button", {
        name: /Won renewal|Lost renewal/,
      }),
    ).toBeNull();
    if (employers.length) {
      const candidates = within(dialog).getAllByRole("button", {
        name: /Employer [AB] renewal|Partner renewal/,
      });
      expect(candidates).toHaveLength(3);
      expect(candidates[0]?.textContent).toContain("Employer A renewal");
      expect(candidates[1]?.textContent).toContain("Employer B renewal");
      expect(candidates[2]?.textContent).toContain("Partner renewal");
    }
    await user.click(
      within(dialog).getByRole("button", { name: /Partner renewal/ }),
    );
    expect(
      within(dialog).getByRole("button", { name: "Create" }),
    ).toHaveProperty("disabled", false);
    await user.type(input, "different");
    expect(
      within(dialog).getByRole("button", { name: "Create" }),
    ).toHaveProperty("disabled", true);
  },
);
