// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ProductReports } from "./reporting.productpanel";

// One entry per live product, each opening Sales review for it, and the panel
// with no product to list.

const meta: Meta = {
  title: "Analytics/Reporting/Product reports",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function panel(products: readonly { id: string; name: string }[]) {
  installFetchStub({
    "GET /products": () =>
      jsonResponse({
        data: products,
        page: { next_cursor: null, has_more: false },
      }),
  });
  return (
    <StoryProviders>
      <ProductReports />
    </StoryProviders>
  );
}

/** The live catalog, one link per product. */
export const Products: Story = {
  render: () =>
    panel([
      { id: "p-suite", name: "Suite" },
      { id: "p-onboarding", name: "Onboarding" },
      { id: "p-support", name: "Support plan" },
    ]),
};

/** A catalog with no live product. */
export const EmptyProductList: Story = { render: () => panel([]) };
