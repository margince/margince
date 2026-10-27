// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMemo } from "react";
import {
  formatSizeBand,
  SIZE_BAND_OPTIONS,
} from "../../components/companylookups";
import type { ListQuery, ListState } from "../../components/listquery";
import type { ListChip } from "../../design-system/listsurface";
import { countryOptions } from "../../format/assignedcountries";
import { industryOptions } from "../../format/industry";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemStatus } from "../../lib/problem";
import { heldRefusedDials, useReReadMeWhen } from "../../lib/refusedfilters";
import { useMe } from "../../lib/session";

// The lead list's dials that read the linked company: what it does, where it
// is and how big it is. A seat that may not read companies is refused all three.

const COMPANY_DIALS = ["industry", "country", "size_band"] as const;

type CompanyDial = (typeof COMPANY_DIALS)[number];

// The words a lead dial is named by. A refused dial missing here is sent as it
// is, so the list's own refusal says what happened.
const DIAL_NAMES: Readonly<Record<CompanyDial | "list_id", MessageKey>> = {
  industry: "create.industry",
  country: "deals.filterCountry",
  size_band: "lead.companySize",
  list_id: "lists.record.title",
};

function dialNameKey(dial: string): MessageKey | undefined {
  return Object.entries(DIAL_NAMES).find(([known]) => known === dial)?.[1];
}

function dialLabel(dial: CompanyDial, t: Translator): string {
  return t(DIAL_NAMES[dial]);
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

export function namedRefusedDials(
  query: ListQuery,
  withheld: ReadonlySet<string>,
): string[] {
  return heldRefusedDials(query.filters, withheld).filter(
    (dial) => dialNameKey(dial) !== undefined,
  );
}

export function refusedDialNames(
  dials: readonly string[],
  t: Translator,
): string {
  return dials
    .flatMap((dial) => {
      const key = dialNameKey(dial);
      return key ? [t(key)] : [];
    })
    .join(", ");
}

// A 403 names no dial, so it only re-reads /me; the dials /me then refuses are
// named, and a refusal for any other cause stays the list's own error.
export function useRefusalOf<Row>(
  listQuery: ListState<Row>,
  withheld: ReadonlySet<string>,
): { refused: string[]; state: ListState<Row> } {
  const refused = namedRefusedDials(listQuery.query, withheld);
  useReReadMeWhen(
    refused.length === 0 &&
      listQuery.isError &&
      problemStatus(listQuery.error) === 403,
  );
  const state =
    refused.length > 0
      ? {
          ...listQuery,
          rows: [],
          isPending: false,
          isError: false,
          hasMore: false,
          total: 0,
        }
      : listQuery;
  return { refused, state };
}
