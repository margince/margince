/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { LocaleProvider } from "../i18n";
import { CompaniesScreen, CompanyScreen } from "./companies";
import {
  company,
  company360,
  jsonResponse,
  stubFetch,
} from "./company.fixtures";
import { mount } from "./contactpage.testkit";
import { WriteToHost } from "./writeto";

// One record, one mark: the list a reader found it in, the card another record
// lists it on and its own page all key the chip on the record's id. When two
// surfaces keyed it on different strings, a company was one colour in the list
// and another on the page that list opened.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <WriteToHost>
          <RecordShell>{ui}</RecordShell>
        </WriteToHost>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The inline numbers the chip beside `name` draws its mesh from.
function meshBeside(name: string, holder: string): string {
  const chip = screen
    .getAllByText(name)
    .map((node) => node.closest(holder)?.querySelector(".avatar-mesh"))
    .find((node) => node != null);
  const mesh = chip?.getAttribute("style");
  if (!mesh?.includes("--avatar-hue-a")) {
    throw new Error(`no mesh beside ${name} in ${holder}`);
  }
  return mesh;
}

const headMesh = () =>
  meshBeside(
    screen.getByRole("heading", { level: 1 }).textContent ?? "",
    ".record-head",
  );

const noMore = { next_cursor: null, has_more: false };

const dana = {
  contact_id: "p-1",
  full_name: "Dana Buyer",
  title: "VP Procurement",
  strength: { score: 40, bucket: "moderate", factors: {}, inbound_90d: 1 },
  deal_roles: [],
  consent: {},
  routes: { top: [], remainder: 0, untried: false },
};

describe("a record's mark", () => {
  it("is the same in the companies list and on the company's page", async () => {
    stubFetch(async (url) =>
      new URL(url).pathname.endsWith("/companies")
        ? jsonResponse({ data: [company], page: noMore })
        : jsonResponse(url.includes("/activities") ? { data: [] } : company),
    );
    render(<CompaniesScreen />);
    await screen.findByText(company.display_name);
    const listed = meshBeside(company.display_name, ".avatar-row");
    cleanup();

    render(<CompanyScreen id={company.id} />);
    await screen.findByRole("heading", {
      level: 1,
      name: company.display_name,
    });
    expect(headMesh()).toBe(listed);
  });

  it("is the same on the card another record lists it on and on its own page", async () => {
    stubFetch(
      async (url) =>
        jsonResponse(url.includes("/activities") ? { data: [] } : company),
      {
        company360: {
          ...company360,
          contacts: { data: [dana], page: noMore },
        },
      },
    );
    render(<CompanyScreen id={company.id} />);
    await screen.findByRole("link", { name: dana.full_name });
    const carded = meshBeside(dana.full_name, ".record-card");
    cleanup();
    vi.unstubAllGlobals();

    mount("overview");
    await screen.findByRole("heading", { level: 1, name: dana.full_name });
    expect(headMesh()).toBe(carded);
  });
});
