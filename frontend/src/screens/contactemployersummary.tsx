import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Avatar } from "../design-system/atoms";
import { CompanyLogo } from "../design-system/companylogo";
import { formatNumber } from "../format/format";
import { type Locale, translatePlural, type useT } from "../i18n";
import { throwProblem } from "./common";
import type { Employment } from "./contactemployers";

type Company = components["schemas"]["Company"];

// The row's logo and its contact/open-deal counts are facts about the
// COMPANY, not about the employment edge, so they come off the company
// record rather than off Contact360Employment. Cached for five minutes: a
// reader scrolling a long employment list re-renders the same employer's row
// far more often than the company itself changes.
//
// A company this reader may not open (403) or that has since gone (404)
// renders the row without a logo or counts rather than throwing: the same
// degrade `entityref.tsx`'s `unnamedOrThrow` gives a reference nobody may
// follow. Every other failure still throws, so react-query holds it as an
// error instead of a silently blank row.
export function useEmployerSummary(companyId: string) {
  return useQuery({
    queryKey: ["company", companyId],
    queryFn: async (): Promise<Company | null> => {
      const { data, error, response } = await api.GET("/companies/{id}", {
        params: { path: { id: companyId } },
      });
      if (error) {
        if (response.status === 403 || response.status === 404) {
          return null;
        }
        throwProblem(error);
      }
      return data;
    },
    staleTime: 5 * 60_000,
  });
}

// The counts line under the company name: contacts, then open deals, joined
// the way the rest of this rail joins two short facts on one line. Callers
// gate on `contact_count`/`open_deal_count` being present at all: a company
// the reader lacks the grants to see either number for renders neither.
export function employerFacts(
  company: Pick<Company, "contact_count" | "open_deal_count">,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  const parts: string[] = [];
  if (company.contact_count !== undefined) {
    parts.push(
      translatePlural(
        locale,
        "contact.employer.contacts",
        company.contact_count,
        { count: formatNumber(company.contact_count, locale) },
      ),
    );
  }
  if (company.open_deal_count !== undefined) {
    parts.push(
      company.open_deal_count === 0
        ? t("contact.employer.noOpenDeals")
        : translatePlural(
            locale,
            "contact.employer.openDeals",
            company.open_deal_count,
            { count: formatNumber(company.open_deal_count, locale) },
          ),
    );
  }
  return parts.join(" · ");
}

// Whether the row has anything to say in its facts line at all: a company
// this reader may not open, or one the read has not answered yet, has none.
export function hasEmployerFacts(company: Company | null | undefined): boolean {
  return (
    company != null &&
    (company.contact_count !== undefined ||
      company.open_deal_count !== undefined)
  );
}

// The employer's mark, or an empty slot the same size: a second row naming
// the same company (`show` false) carries no mark of its own, but the grid
// column stays so every row's name and role line up under one another.
export function EmploymentLogo({
  employment,
  company,
  show,
  t,
}: Readonly<{
  employment: Employment;
  company: Company | null | undefined;
  show: boolean;
  t: ReturnType<typeof useT>;
}>) {
  if (!show) {
    return <span className="pe-employment-logo" />;
  }
  const name = employment.company_name ?? t("field.unset");
  return (
    <span className="pe-employment-logo">
      <CompanyLogo
        name={name}
        src={company?.logo_url}
        fallback={
          <Avatar
            identity={employment.company_id}
            name={name}
            shape="company"
            size="sm"
          />
        }
      />
    </span>
  );
}
