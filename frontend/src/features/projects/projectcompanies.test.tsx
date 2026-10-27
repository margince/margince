/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { type Locale, LocaleProvider } from "../../i18n";
import { vi } from "../../i18n/vi";
import { parseHash } from "../../lib/router";
import { ProjectCompanies } from "./projectcompanies";

// Every OTHER link in this tree spells a company `#/companies/<id>`; this
// section is the one place that builds the href itself rather than taking
// ProjectLinks' `#/projects/<id>` default, which is how it once spelled a
// prefix the router does not answer. The oracle is parseHash — the router's own
// entry point — so the assertion cannot drift from the route table the way a
// hard-coded list of prefixes would.
function renderSection(
  companies: readonly unknown[],
  readOnly = true,
  locale: Locale = "en",
) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial={locale}>
        <ProjectCompanies
          projectId="01a02e25-a5ac-7099-8099-581cbf001a99"
          companies={
            companies as Parameters<typeof ProjectCompanies>[0]["companies"]
          }
          readOnly={readOnly}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the companies on a project", () => {
  const netcare = {
    company_id: "01a02be9-2293-75d2-9dd2-3027d9b63dc2",
    display_name: "netcare",
    role: "customer",
  };

  it("links a company to an address the router answers", () => {
    renderSection([netcare]);

    // EVERY link the section draws for this company, not the first one: a
    // section that renders two must not pass on the half that happens to work.
    const links = screen.getAllByRole("link", { name: /netcare/ });
    expect(links.length).toBeGreaterThan(0);

    // The defect this holds: `#/companies/<id>` parses to not-found, so the
    // row rendered fine and the click landed on an empty page.
    for (const link of links) {
      const route = parseHash(link.getAttribute("href") ?? "");
      expect(route.screen).not.toBe("not-found");
      expect(route.id).toBe(netcare.company_id);
    }
  });

  // Every card carries the same "Detach" item, so the name has to say which
  // company it takes off.
  it("names the company on each card's detach item", async () => {
    const user = userEvent.setup();
    renderSection([netcare], false);

    await user.click(screen.getByRole("button", { name: "More actions" }));

    expect(
      screen.getByRole("button", { name: "Detach netcare" }),
    ).toBeInTheDocument();
  });

  it("names each company's role in the reader's language", () => {
    renderSection(
      [
        netcare,
        {
          ...netcare,
          company_id: "c-2",
          display_name: "relay",
          role: "reseller",
        },
      ],
      true,
      "vi",
    );

    expect(screen.getByText(vi["projectRole.customer"])).toBeInTheDocument();
    expect(screen.queryByText("customer")).not.toBeInTheDocument();
    // A role this build has no words for reads as itself, not as a partner.
    expect(screen.getByText("reseller")).toBeInTheDocument();
  });
});
