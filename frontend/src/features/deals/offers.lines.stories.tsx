import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { OfferLineEditor } from "./offers.lines";

type Offer = components["schemas"]["Offer"];

const meta: Meta = {
  title: "Records/Offers/Lines",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const consultingLine = {
  id: "li-1",
  position: 1,
  product_id: "p-1",
  description: "Consulting hours",
  unit: "hour",
  quantity: 10,
  unit_price_minor: 10000,
  discount_pct: 0,
  tax_rate: 19,
  tax_category: "standard" as const,
  line_net_minor: 100000,
  line_tax_minor: 19000,
  line_total_minor: 119000,
  evidence: null,
  price_grounded: true,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const draftOffer: Offer = {
  id: "o-1",
  deal_id: "d-1",
  offer_number: "ANG-2026-0007",
  revision: 1,
  status: "draft",
  currency: "EUR",
  net_minor: 150000,
  tax_minor: 27000,
  gross_minor: 177000,
  line_items: [
    consultingLine,
    {
      ...consultingLine,
      id: "li-2",
      position: 2,
      product_id: null,
      description: "Old contract, typed rate",
      unit_price_minor: 5000,
      tax_rate: 16,
      tax_category: null,
      line_net_minor: 50000,
      line_tax_minor: 8000,
      line_total_minor: 58000,
    },
  ],
  source: "manual",
  ai_generated: false,
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

/** A line that takes the sheet's standard rate, beside one with a typed rate. */
export const CategoryAndTypedRates: Story = {
  render: () => {
    installFetchStub({ "GET /products": () => jsonResponse(emptyPage) });
    return (
      <StoryProviders>
        <OfferLineEditor offer={draftOffer} />
      </StoryProviders>
    );
  },
};
