// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { TaxRatesCard } from "./taxrates";

// Settings › Company › the tax rate sheet, by country and category.

const rate = (id: string, extra: Record<string, unknown>) => ({
  id,
  country: "de",
  category: "standard",
  rate: "19.00",
  valid_from: "2026-01-01",
  created_at: "2026-01-01T08:00:00Z",
  ...extra,
});

const RATES = [
  rate("tr-1", {}),
  rate("tr-2", { category: "reduced", rate: "7.00" }),
  rate("tr-3", { country: "at", rate: "20.00" }),
];

function story(allow: Parameters<typeof meRoute>[0], rows: object[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /tax-rates": () => jsonResponse({ data: rows }),
    });
    return (
      <StoryProviders>
        <TaxRatesCard />
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/Company/Company profile/Tax rates",
  component: TaxRatesCard,
} satisfies Meta<typeof TaxRatesCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Keeper: Story = {
  render: story({ tax_rate: ["read", "create"] }, RATES),
};
export const Reader: Story = { render: story({ tax_rate: ["read"] }, RATES) };
export const Empty: Story = {
  render: story({ tax_rate: ["read", "create"] }, []),
};
export const Withheld: Story = { render: story({}, []) };
