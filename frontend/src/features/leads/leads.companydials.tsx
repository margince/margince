// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { type Dispatch, type SetStateAction, useEffect, useMemo } from "react";
import {
  formatSizeBand,
  SIZE_BAND_OPTIONS,
} from "../../components/companylookups";
import type { ListQuery, ListState } from "../../components/listquery";
import { Button } from "../../design-system/atoms";
import type { ListChip } from "../../design-system/listsurface";
import { countryOptions } from "../../format/assignedcountries";
import { industryOptions } from "../../format/industry";
import { type Locale, type Translator, useT } from "../../i18n";
import { withoutWithheldDials } from "../../lib/dealranges";
import { problemStatus } from "../../lib/problem";
import { useMe } from "../../lib/session";

// The lead list's dials that read the linked company: what it does, where it
// is and how big it is. A seat that may not read companies is refused all three.

const COMPANY_DIALS = ["industry", "country", "size_band"] as const;

export type CompanyDial = (typeof COMPANY_DIALS)[number];

function dialLabel(dial: CompanyDial, t: Translator): string {
  switch (dial) {
    case "industry":
      return t("create.industry");
    case "country":
      return t("deals.filterCountry");
    case "size_band":
      return t("lead.companySize");
  }
}

export function useWithheldLeadDials(): ReadonlySet<string> {
  const refused = useMe().data?.authorization?.refused_lead_filters;
  return useMemo(() => new Set(refused ?? []), [refused]);
}

export function leadCompanyChips(
  t: Translator,
  locale: Locale,
  withheld: ReadonlySet<string>,
): ListChip[] {
  const chips: Record<CompanyDial, ListChip> = {
    industry: {
      key: "industry",
      label: dialLabel("industry", t),
      allLabel: t("lead.filterIndustryAll"),
      options: industryOptions(t, locale),
      filterable: true,
    },
    country: {
      key: "country",
      label: dialLabel("country", t),
      allLabel: t("deals.filterCountryAll"),
      options: countryOptions(locale),
      filterable: true,
    },
    size_band: {
      key: "size_band",
      label: dialLabel("size_band", t),
      allLabel: t("lead.filterCompanySizeAll"),
      options: SIZE_BAND_OPTIONS.map((band) => ({
        value: band,
        label: formatSizeBand(t, band),
      })),
    },
  };
  return COMPANY_DIALS.filter((dial) => !withheld.has(dial)).map(
    (dial) => chips[dial],
  );
}

// A refusal /me did not foresee names no dial, so every company dial the
// refused query held is the suspect.
export function refusedLeadDials(
  query: ListQuery,
  withheld: ReadonlySet<string>,
  error: unknown,
): CompanyDial[] {
  const held = COMPANY_DIALS.filter((dial) => Boolean(query.filters[dial]));
  const named = held.filter((dial) => withheld.has(dial));
  if (named.length > 0) {
    return named;
  }
  return problemStatus(error) === 403 ? held : [];
}

export function refusedDialNames(
  dials: readonly CompanyDial[],
  t: Translator,
): string {
  return dials.map((dial) => dialLabel(dial, t)).join(", ");
}

export function sendableLeadQuery(
  query: ListQuery,
  withheld: ReadonlySet<string>,
): ListQuery {
  const filters = withoutWithheldDials(query.filters, withheld);
  return filters === query.filters ? query : { ...query, filters };
}

// No rows rather than rows the refused dial did not narrow. A refusal /me did
// not foresee reads /me again, which then withholds the dial.
export function useRefusalOf<Row>(
  listQuery: ListState<Row>,
  withheld: ReadonlySet<string>,
): { refused: CompanyDial[]; state: ListState<Row> } {
  const refused = refusedLeadDials(listQuery.query, withheld, listQuery.error);
  const staleRefusal = listQuery.isError && refused.length > 0;
  const queryClient = useQueryClient();
  useEffect(() => {
    if (staleRefusal) {
      queryClient.invalidateQueries({ queryKey: ["me"] });
    }
  }, [staleRefusal, queryClient]);
  const state =
    refused.length > 0
      ? { ...listQuery, rows: [], isError: false, hasMore: false, total: 0 }
      : listQuery;
  return { refused, state };
}

export function RefusedDialsNote({
  dials,
  setQuery,
}: Readonly<{
  dials: readonly CompanyDial[];
  setQuery: Dispatch<SetStateAction<ListQuery>>;
}>) {
  const t = useT();
  const dial = refusedDialNames(dials, t);
  return (
    <span>
      {t("lead.filterRefused", { dial })}{" "}
      <Button
        onClick={() =>
          setQuery((query) => ({
            ...query,
            filters: Object.fromEntries(
              Object.entries(query.filters).filter(
                ([key]) => !dials.some((dial) => dial === key),
              ),
            ),
          }))
        }
      >
        {t("lead.filterRefusedRemove", { dial })}
      </Button>
    </span>
  );
}
