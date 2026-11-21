// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useCallback } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { formatMoney } from "../format/format";
import { unitLabel } from "../format/productunit";
import { type Locale, type Translator, translate, useLocale } from "../i18n";
import { unwrap } from "../lib/problem";

type Product = components["schemas"]["Product"];

/** A catalog product as a picker offers it, with the price a line takes from it. */
export type ProductCandidate = RecordPickerCandidate &
  Readonly<{ unitPriceMinor: number; currency: string }>;

async function matchingProducts(q: string): Promise<Product[]> {
  const data = unwrap(
    await api.GET("/products", {
      params: { query: { q, limit: 10 } },
    }),
  );
  return data.data;
}

/** The catalog's products whose name matches q, for a picker. */
export async function searchProductCandidates(
  q: string,
): Promise<{ id: string; name: string }[]> {
  return (await matchingProducts(q)).map((p) => ({ id: p.id, name: p.name }));
}

// Two products may share a name, or differ only in currency.
function productDetail(product: Product, t: Translator, locale: Locale) {
  const price = t("product.candidatePrice", {
    price: formatMoney(product.unit_price_minor, product.currency, locale),
    unit: unitLabel(product.unit, t),
  });
  return product.sku ? `${product.sku} · ${price}` : price;
}

/**
 * The product search for a line picker. Each candidate shows its SKU and rate
 * card price under its name, and carries that price for the line to start from.
 */
export function useProductCandidateSearch() {
  const { locale } = useLocale();
  return useCallback(
    async (q: string): Promise<ProductCandidate[]> => {
      const t: Translator = (key, params) => translate(locale, key, params);
      return (await matchingProducts(q)).map((product) => ({
        id: product.id,
        name: product.name,
        detail: productDetail(product, t, locale),
        unitPriceMinor: product.unit_price_minor,
        currency: product.currency,
      }));
    },
    [locale],
  );
}

/**
 * The name of the product a record names by id. A record keeps only the id,
 * and every surface that draws it reads the name the same way.
 */
export function useProductName(id: string | null | undefined) {
  return useQuery({
    queryKey: ["product", id],
    enabled: Boolean(id),
    queryFn: async () =>
      unwrap(
        await api.GET("/products/{id}", { params: { path: { id: id ?? "" } } }),
      ).name,
  });
}
