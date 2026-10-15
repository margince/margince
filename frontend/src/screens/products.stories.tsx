import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { meFixture } from "../testing/mefixture";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { ProductsAdmin } from "./products";

const meta: Meta = {
  title: "Settings/Sales/Products and offers/Products",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const product = {
  id: "p-1",
  name: "Consulting Day",
  sku: "CONS-DAY",
  unit: "day",
  unit_price_minor: 150000,
  currency: "EUR",
  default_tax_rate: 19,
  active: true,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

// One of each billing and status, so every Price, Billing and Status cell shows.
const catalogue = [
  { ...product, billing_model: "one_time" },
  {
    ...product,
    id: "p-2",
    name: "Support Plan",
    sku: "SUP-M",
    unit_price_minor: 9900,
    billing_model: "recurring",
    billing_interval_months: 1,
  },
  {
    ...product,
    id: "p-3",
    name: "Annual Licence",
    sku: null,
    unit_price_minor: 1200000,
    billing_model: "recurring",
    billing_interval_months: 12,
    active: false,
  },
  {
    ...product,
    id: "p-4",
    name: "Legacy Workshop",
    archived_at: "2026-06-02T08:00:00Z",
  },
];

const catalogueRead = () =>
  jsonResponse({
    data: catalogue,
    page: { next_cursor: null, has_more: false },
  });

// Every story here needs a principal, because the screen's write affordances are
// gated on product grants now. The stub REFUSES to answer an unrouted `GET /me`
// rather than guessing one — so without
// this the whole catalog captured the read-only posture and no story showed the
// editor. Named once rather than repeated per story.
const AUTHORING_ME = () =>
  jsonResponse(
    meFixture({ allow: { product: ["read", "create", "update", "delete"] } }),
  );

function renderCatalogue() {
  installFetchStub({ "GET /me": AUTHORING_ME, "GET /products": catalogueRead });
  return (
    <StoryProviders>
      <ProductsAdmin />
    </StoryProviders>
  );
}

// The create verb sits in the panel's head, on a phone as well.
async function pressHeadVerb(canvasElement: HTMLElement, name: string) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", { name }),
  );
}

// One page of rows: no pager and no page-size control under them.
export const List: Story = {
  render: renderCatalogue,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("row", { name: /Support Plan/ });
    await expect(
      canvas.queryByRole("navigation", { name: "Pages" }),
    ).toBeNull();
    await expect(
      canvas.queryByRole("combobox", { name: "Rows per page" }),
    ).toBeNull();
  },
};

// A short list keeps only the archived switch, flush under the intro's interval.
export const ShortWithArchivedSwitch: Story = {
  render: renderCatalogue,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("row", { name: /Support Plan/ });
    await waitFor(() => {
      const tools = canvas
        .getByRole("checkbox", { name: "Show archived" })
        .closest<HTMLElement>(".lt-tools");
      expect(tools && getComputedStyle(tools).paddingTop).toBe("0px");
    });
  },
};

export const RowMenu: Story = {
  render: renderCatalogue,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Support Plan" }),
    );
    await within(document.body).findByRole("button", {
      name: "Archive product",
    });
  },
};

// The billing choices read as words, never as their catalog keys.
export const NewProductBilling: Story = {
  render: renderCatalogue,
  play: async ({ canvasElement }) => {
    await pressHeadVerb(canvasElement, "New product");
    const form = within(await within(document.body).findByRole("dialog"));
    await userEvent.click(
      form.getByRole("combobox", { name: /^Billing period/ }),
    );
    await within(document.body).findByRole("option", { name: "Quarterly" });
  },
};
// At 390px each row is a card: the name heads it and the "…" sits beside it.
export const ListPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: renderCatalogue,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("button", { name: "Actions for Support Plan" });
    await expect(
      canvas.getByRole("button", { name: "New product" }),
    ).toBeVisible();
  },
};
// No rows: one line under the intro, with no headings, dials or count.
export const Empty: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("No products yet.")).toBeVisible();
    await expect(canvas.queryByRole("columnheader")).toBeNull();
    await expect(canvas.queryByRole("button", { name: /^Sort/ })).toBeNull();
  },
  render: () => {
    installFetchStub({
      "GET /me": AUTHORING_ME,
      "GET /products": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <ProductsAdmin />
      </StoryProviders>
    );
  },
};
export const LoadError: Story = {
  render: () => {
    installFetchStub({
      "GET /me": AUTHORING_ME,
      "GET /products": () =>
        jsonResponse(
          { title: "server error", detail: "products unavailable" },
          500,
        ),
    });
    return (
      <StoryProviders>
        <ProductsAdmin />
      </StoryProviders>
    );
  },
};

export const EmptyPhone: Story = {
  ...Empty,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
