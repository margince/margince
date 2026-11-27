/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { steppedClock } from "../../testing/steppedclock";
import {
  ImportedEmploymentHistory,
  useEmploymentImport,
} from "./employmentimport";

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

function History() {
  const imported = useEmploymentImport(view, true);
  return <ImportedEmploymentHistory view={view} canEdit imported={imported} />;
}

// Serves one imported episode waiting for a match, and records each write.
function serve(domain: string) {
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
            domain,
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
        <History />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return writes;
}

it.each(["keep", "Current", "Former"])(
  "resolves provider uncertainty with %s status",
  async (choice) => {
    const user = steppedClock();
    const writes = serve("buyer.example");
    await user.click(
      await screen.findByRole("button", {
        name: englishCopy["employment.resolve"],
      }),
    );
    await screen.findByRole("dialog", {
      name: englishCopy["employment.resolve"],
    });
    const status = screen.getByRole("combobox", {
      name: englishCopy["employment.statusLabel"],
    });
    expect(status.textContent).toContain(
      englishCopy["employment.keepImportedStatus"],
    );
    await user.click(status);
    expect(
      screen.queryByRole("option", {
        name: englishCopy["employment.status.unknown"],
      }),
    ).toBeNull();
    if (choice === "keep") await user.keyboard("{Escape}");
    else await user.click(screen.getByRole("option", { name: choice }));
    await user.click(
      screen.getByRole("button", { name: englishCopy["employment.saveMatch"] }),
    );
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

it("says the website is required with no company picked, and sends nothing", async () => {
  const user = steppedClock();
  const writes = serve("");
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["employment.resolve"],
    }),
  );
  await screen.findByRole("dialog", {
    name: englishCopy["employment.resolve"],
  });
  await user.click(
    screen.getByRole("button", { name: englishCopy["employment.saveMatch"] }),
  );
  expect(
    screen.getByText("Confirmed company website is required."),
  ).toBeTruthy();
  expect(document.activeElement).toBe(
    screen.getByRole("textbox", { name: /^Confirmed company website/ }),
  );
  expect(writes).toHaveLength(0);
});
