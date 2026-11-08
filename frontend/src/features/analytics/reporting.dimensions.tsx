// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import { Badge, Button, Field } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { TokenList } from "../../design-system/tokeninput";
import { countryOptions } from "../../format/assignedcountries";
import { formatAnalyticsNumber } from "../../format/format";
import { INDUSTRY_CODES, industryLabelKey } from "../../format/industry";
import { type Translator, useLocale, useT } from "../../i18n";
import { useSalesInitiatives } from "./reporting.initiatives.queries";
import type { ReportingSelection } from "./reporting.model";
import { VALUE_BASES, valueBasisOf } from "./reporting.selection";

export {
  VALUE_BASES,
  type ValueBasis,
  valueBasisOf,
} from "./reporting.selection";

/**
 * The product, country, industry and value basis of a selection: the dials
 * that narrow one report to a product or a market. Every figure, chart,
 * evidence row and export reads the same four, so a chip never sits above a
 * figure that ignores it.
 */

/** The dials that narrow a report to a product or a market. */
export type Dimensions = Pick<
  ReportingSelection,
  "product_id" | "initiative_id" | "country" | "industry" | "value_basis"
>;

export function dimensionsOf(selection: ReportingSelection): Dimensions {
  return {
    product_id: selection.product_id,
    initiative_id: selection.initiative_id,
    country: selection.country,
    industry: selection.industry,
    value_basis: selection.value_basis,
  };
}

function sameDimensions(a: Dimensions, b: Dimensions): boolean {
  return (
    a.product_id === b.product_id &&
    a.initiative_id === b.initiative_id &&
    a.country === b.country &&
    a.industry === b.industry &&
    (a.value_basis ?? "all") === (b.value_basis ?? "all")
  );
}

/** Moves the dimension dials to `next` when the selection changed them. */
export function applyDimensions(
  filters: Readonly<{
    dimensions: Dimensions;
    setDimensions: (next: Dimensions) => void;
  }>,
  next: ReportingSelection,
): void {
  const wanted = dimensionsOf(next);
  if (!sameDimensions(filters.dimensions, wanted))
    filters.setDimensions(wanted);
}

const DIMENSION_KEYS = [
  "product_id",
  "initiative_id",
  "country",
  "industry",
  "value_basis",
] as const;
type DimensionKey = (typeof DIMENSION_KEYS)[number];

/** The products a reader may narrow a report to: the live, active catalog. */
export function useProductOptions(enabled = true) {
  return useQuery({
    queryKey: ["reporting-products"],
    queryFn: async () => {
      const { data, error } = await api.GET("/products", {
        params: { query: { active: true, limit: 200, sort: "name" } },
      });
      if (error) throw error;
      return data.data.map((product) => ({
        value: product.id,
        label: product.name,
      }));
    },
    enabled,
    staleTime: 60_000,
  });
}

/** How many dimensions the selection narrows by. */
export function dimensionCount(selection: ReportingSelection): number {
  return DIMENSION_KEYS.filter((key) =>
    key === "value_basis"
      ? !!selection.value_basis && selection.value_basis !== "all"
      : !!selection[key],
  ).length;
}

/** Whether the selection narrows by any dimension. */
export function dimensioned(selection: ReportingSelection): boolean {
  return dimensionCount(selection) > 0;
}

/** The selection with every dimension cleared; period, scope and pipeline stay. */
export function withoutDimensions(
  selection: ReportingSelection,
): ReportingSelection {
  const next = { ...selection };
  for (const key of DIMENSION_KEYS) delete next[key];
  return next;
}

export function DimensionFilters({
  selection,
  onChange,
}: Readonly<{
  selection: ReportingSelection;
  onChange: (selection: ReportingSelection) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const products = useProductOptions();
  const initiatives = useSalesInitiatives();
  return (
    <>
      <Field label={t("reporting.initiative")}>
        {(field) => (
          <Select
            {...field}
            value={selection.initiative_id ?? ""}
            options={[
              { value: "", label: t("reporting.allInitiatives") },
              ...(initiatives.data ?? [])
                .filter((initiative) => initiative.status !== "draft")
                .map((initiative) => ({
                  value: initiative.id,
                  label: initiative.name,
                })),
            ]}
            onChange={(value) =>
              onChange({ ...selection, initiative_id: value || undefined })
            }
          />
        )}
      </Field>
      <Field label={t("reporting.product")}>
        {(field) => (
          <Select
            {...field}
            value={selection.product_id ?? ""}
            options={[
              { value: "", label: t("reporting.allProducts") },
              ...(products.data ?? []),
            ]}
            onChange={(value) =>
              onChange({ ...selection, product_id: value || undefined })
            }
          />
        )}
      </Field>
      <Field label={t("reporting.country")}>
        {(field) => (
          <Select
            {...field}
            value={selection.country ?? ""}
            options={[
              { value: "", label: t("reporting.allCountries") },
              ...countryOptions(locale),
            ]}
            onChange={(value) =>
              onChange({ ...selection, country: value || undefined })
            }
          />
        )}
      </Field>
      <Field label={t("reporting.industry")}>
        {(field) => (
          <Select
            {...field}
            value={selection.industry ?? ""}
            options={[
              { value: "", label: t("reporting.allIndustries") },
              ...INDUSTRY_CODES.map((code) => ({
                value: code,
                label: industryName(code, t),
              })),
            ]}
            onChange={(value) =>
              onChange({ ...selection, industry: value || undefined })
            }
          />
        )}
      </Field>
      <Field label={t("reporting.valueBasis")}>
        {(field) => (
          <Select
            {...field}
            value={selection.value_basis ?? "all"}
            options={VALUE_BASES.map((basis) => ({
              value: basis,
              label: t(`reporting.basis.${basis}`),
            }))}
            onChange={(value) => {
              const basis = valueBasisOf(value);
              onChange({
                ...selection,
                value_basis: basis === "all" ? undefined : basis,
              });
            }}
          />
        )}
      </Field>
    </>
  );
}

/**
 * The dimension dials behind one toggle, so a page's line of chips holds its
 * main dials. A selection that already narrows opens folded out, so no active
 * filter hides.
 */
export function FoldedDimensions({
  selection,
  onChange,
}: Readonly<{
  selection: ReportingSelection;
  onChange: (selection: ReportingSelection) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const active = dimensionCount(selection);
  const [open, setOpen] = useState(active > 0);
  return (
    <>
      <Button aria-expanded={open} onClick={() => setOpen(!open)}>
        {active > 0
          ? t("reporting.filtersActive", {
              active: formatAnalyticsNumber(active, locale),
            })
          : t("reporting.filtersMore")}
        <ChevronDown aria-hidden />
      </Button>
      {open && <DimensionFilters selection={selection} onChange={onChange} />}
    </>
  );
}

function industryName(code: string, t: Translator): string {
  const key = industryLabelKey(code);
  return key ? t(key) : code;
}

/**
 * The profile the selection reads under and the dimensions it holds, each
 * chip removable on its own. The product's name comes from the answered
 * context when there is one, so the chip never waits on the catalog read.
 */
export function DimensionChips({
  selection,
  productLabel,
  initiativeLabel,
  onChange,
}: Readonly<{
  selection: ReportingSelection;
  productLabel?: string;
  initiativeLabel?: string;
  onChange: (selection: ReportingSelection) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const products = useProductOptions(!!selection.product_id && !productLabel);
  const initiatives = useSalesInitiatives(
    !!selection.initiative_id && !initiativeLabel,
  );
  const items: { id: DimensionKey; label: string }[] = [];
  if (selection.initiative_id) {
    const name =
      initiativeLabel ??
      initiatives.data?.find((row) => row.id === selection.initiative_id)
        ?.name ??
      t("reporting.initiativeUnnamed");
    items.push({
      id: "initiative_id",
      label: t("analytics.overview.chipInitiative", { name }),
    });
  }
  if (selection.product_id) {
    const name =
      productLabel ??
      products.data?.find((option) => option.value === selection.product_id)
        ?.label ??
      t("reporting.productUnnamed");
    items.push({
      id: "product_id",
      label: t("analytics.overview.chipProduct", { name }),
    });
  }
  if (selection.country) {
    const name =
      countryOptions(locale).find(
        (option) => option.value === selection.country,
      )?.label ?? selection.country;
    items.push({
      id: "country",
      label: t("analytics.overview.chipCountry", { name }),
    });
  }
  if (selection.industry) {
    items.push({
      id: "industry",
      label: t("analytics.overview.chipIndustry", {
        name: industryName(selection.industry, t),
      }),
    });
  }
  if (selection.value_basis && selection.value_basis !== "all") {
    items.push({
      id: "value_basis",
      label: t("analytics.overview.chipBasis", {
        basis: t(`reporting.basis.${selection.value_basis}`),
      }),
    });
  }
  return (
    <>
      <Badge>
        {selection.initiative_id
          ? t("analytics.overview.profileInitiative")
          : selection.product_id
            ? t("analytics.overview.profileProduct")
            : t("analytics.overview.profileCompany")}
      </Badge>
      {items.length > 0 && (
        <TokenList
          items={items}
          removeLabel={(item) =>
            t("analytics.overview.removeFilter", { filter: item.label })
          }
          onRemove={(id) => {
            const next = { ...selection };
            const key = DIMENSION_KEYS.find((candidate) => candidate === id);
            if (key) delete next[key];
            onChange(next);
          }}
        />
      )}
    </>
  );
}
