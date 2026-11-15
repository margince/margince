/** @vitest-environment happy-dom */
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { CompaniesScreen } from "../features/companies/companies.list";
import { CompanyContactsList } from "../features/companies/companycontacts/contacts";
import { ContactsScreen } from "../features/contacts/contacts";
import { OfferTemplatesAdmin } from "../features/deals/offertemplates";
import { ProductsAdmin } from "../features/deals/products";
import { LeadsScreen } from "../features/leads/leads.list";
import { PartnersScreen } from "../features/projects/partners";
import { ProjectsScreen } from "../features/projects/projects";
import { cancellationClient, type HeldRead, holdReads } from "./cancellation";

let held: HeldRead[] = [];
afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const read of held)
      read.answer({ data: [], page: { has_more: false } });
  });
  vi.restoreAllMocks();
  window.history.replaceState(null, "", "#/");
});

it.each([
  { route: "companies", path: "/v1/companies", screen: <CompaniesScreen /> },
  {
    route: "companies/c-1",
    path: "/v1/companies/c-1/contacts",
    screen: <CompanyContactsList companyId="c-1" />,
  },
  { route: "contacts", path: "/v1/contacts", screen: <ContactsScreen /> },
  { route: "leads", path: "/v1/leads", screen: <LeadsScreen /> },
  { route: "projects", path: "/v1/projects", screen: <ProjectsScreen /> },
  { route: "partners", path: "/v1/partners", screen: <PartnersScreen /> },
  { route: "products", path: "/v1/products", screen: <ProductsAdmin /> },
  {
    route: "offer-templates",
    path: "/v1/offer-templates",
    screen: <OfferTemplatesAdmin />,
  },
])(
  "aborts the actual $route list HTTP request when its screen closes",
  async ({ route, path, screen }) => {
    window.history.replaceState(null, "", `#/${route}`);
    held = holdReads((url) => url.pathname === path);
    const { client, wrapper } = cancellationClient();
    const view = render(screen, { wrapper });
    await waitFor(() => expect(held.length).toBeGreaterThan(0));
    expect(held.every((read) => !read.request.signal.aborted)).toBe(true);
    view.unmount();
    expect(held.every((read) => read.request.signal.aborted)).toBe(true);
    client.clear();
  },
);
