/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { steppedClock } from "../../testing/steppedclock";
import { ImportedEmploymentHistory } from "./employmentimport";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  Reflect.deleteProperty(globalThis, "jest");
});

const view: components["schemas"]["Contact360"] = {
  as_of: "2026-10-05T10:00:00Z",
  sections_omitted: [],
  contact: {
    id: "contact",
    full_name: "Sample Contact",
    source: "manual",
    captured_by: "human:sample",
    created_at: "2026-10-05T10:00:00Z",
    updated_at: "2026-10-05T10:00:00Z",
  },
  provider_profiles: [
    {
      provider: "surfe",
      state: "completed",
      categories_not_requested: [],
      emails: [],
      mobile_phones: [],
      departments: [],
      seniorities: [],
      job_history: [{ company_name: "Buyer", job_title: "Advisor" }],
    },
  ],
};

it.each(["keep", "Current", "Former"])(
  "resolves provider uncertainty with %s status",
  async (choice) => {
    const user = steppedClock();
    const writes: components["schemas"]["EmploymentImportRequest"][] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = new Request(input, init);
        if (request.method === "POST") {
          writes.push(await request.json());
          return Response.json({ contact_id: "contact", items: [] });
        }
        return Response.json({
          contact_id: "contact",
          items: [
            {
              key: "episode",
              provider: "surfe",
              company_name: "Buyer",
              role: "Advisor",
              employment_status: "unknown",
              state: "needs_match",
              domain: "buyer.example",
            },
          ],
        });
      }),
    );
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <ImportedEmploymentHistory view={view} canEdit />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await user.click(
      await screen.findByRole("button", { name: "Match company" }),
    );
    await screen.findByRole("dialog", { name: "Match company" });
    const status = screen.getByRole("combobox", { name: "Employment status" });
    expect(status.textContent).toContain("Keep imported status");
    await user.click(status);
    expect(screen.queryByRole("option", { name: "Status unknown" })).toBeNull();
    if (choice === "keep") await user.keyboard("{Escape}");
    else await user.click(screen.getByRole("option", { name: choice }));
    await user.click(screen.getByRole("button", { name: "Save company link" }));
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toMatchObject({
      action: "resolve",
      key: "episode",
      domain: "buyer.example",
    });
    if (choice === "keep")
      expect(writes[0]).not.toHaveProperty("employment_status");
    else expect(writes[0]?.employment_status).toBe(choice.toLowerCase());
  },
);
