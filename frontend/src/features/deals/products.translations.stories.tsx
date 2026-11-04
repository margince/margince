// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ProductTranslationsDialog } from "./products.translations";

// A product's name and description in further languages, from its row menu.

const PRODUCT: components["schemas"]["Product"] = {
  id: "p-1",
  name: "Retainer",
  unit: "unit",
  unit_price_minor: 50000,
  currency: "EUR",
  default_tax_rate: 19,
  active: true,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const meta = {
  title: "Settings/Sales/Products and offers/Product translations",
  component: ProductTranslationsDialog,
  args: { product: PRODUCT, onClose: () => undefined },
} satisfies Meta<typeof ProductTranslationsDialog>;
export default meta;
type Story = StoryObj<typeof meta>;

const dialog = (data: object[]) => () => {
  installFetchStub({
    "GET /products/p-1/translations": () => jsonResponse({ data }),
  });
  return (
    <StoryProviders>
      <ProductTranslationsDialog product={PRODUCT} onClose={() => undefined} />
    </StoryProviders>
  );
};

export const TwoLanguages: Story = {
  render: dialog([
    {
      language: "de",
      name: "Pauschale",
      description: "Monatliche Betreuung",
      version: 1,
      updated_at: "2026-11-01T08:00:00Z",
    },
    {
      language: "fr",
      name: "Forfait",
      version: 1,
      updated_at: "2026-11-01T08:00:00Z",
    },
  ]),
};

export const NoneYet: Story = { render: dialog([]) };
