/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { OfferTemplatesAdmin } from "./offertemplates";
import { ProductsAdmin } from "./products";

const version = 7;
const timestamps = {
  version,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};
const cases = [
  {
    path: "products",
    verb: "Archive product",
    Component: ProductsAdmin,
    record: {
      ...timestamps,
      id: "p-1",
      name: "Support",
      sku: "SUPPORT",
      unit: "day",
      unit_price_minor: 10000,
      currency: "EUR",
      default_tax_rate: 19,
      active: true,
      source: "manual",
      captured_by: "human:u-1",
    },
  },
  {
    path: "offer-templates",
    verb: "Archive template",
    Component: OfferTemplatesAdmin,
    record: {
      ...timestamps,
      id: "t-1",
      name: "Terms",
      locale: "en",
      is_default: false,
      layout: {},
    },
  },
];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it.each(cases)(
  "archives $path at the version the reader reviewed",
  async ({ path, verb, Component, record }) => {
    const user = userEvent.setup();
    const requests: Request[] = [];
    const json = (body: unknown) =>
      new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    vi.stubGlobal(
      "fetch",
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(input, init);
        if (request.url.endsWith("/me"))
          return json(
            meFixture({
              allow: {
                product: ["read", "delete"],
                offer_template: ["read", "delete"],
              },
            }),
          );
        if (request.method === "DELETE") {
          requests.push(request);
          return json(record);
        }
        return json({
          data: [record],
          page: { next_cursor: null, has_more: false },
        });
      },
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <Component />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await user.click(
      await screen.findByRole("button", { name: `Actions for ${record.name}` }),
    );
    await user.click(screen.getByRole("button", { name: verb }));
    const dialog = await screen.findByRole("dialog");
    expect(requests).toHaveLength(0);
    await user.click(within(dialog).getByRole("button", { name: verb }));
    await waitFor(() => expect(requests).toHaveLength(1));
    expect(new URL(requests[0].url).pathname).toBe(`/v1/${path}/${record.id}`);
    expect(requests[0].headers.get("If-Match")).toBe(String(version));
  },
);
