// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ProductField } from "./webformproduct";

// The product every lead a web form captures asks about.

const meta: Meta = {
  title: "Settings/Web forms/Product field",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function Field({ initial }: Readonly<{ initial: string }>) {
  const [productId, setProductId] = useState(initial);
  return <ProductField productId={productId} onChange={setProductId} />;
}

function field(initial: string) {
  installFetchStub({
    "GET /products/p-suite": () =>
      jsonResponse({ id: "p-suite", name: "Suite" }),
    "GET /products": () =>
      jsonResponse({
        data: [{ id: "p-suite", name: "Suite" }],
        page: { next_cursor: null, has_more: false },
      }),
  });
  return (
    <StoryProviders>
      <Field initial={initial} />
    </StoryProviders>
  );
}

/** A form whose leads all ask about one product. */
export const Chosen: Story = { render: () => field("p-suite") };

/** A form that names no product. */
export const NoneChosen: Story = { render: () => field("") };
