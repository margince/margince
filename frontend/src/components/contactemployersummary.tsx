import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Avatar } from "../design-system/atoms";
import { CompanyLogo } from "../design-system/companylogo";
import { formatNumber } from "../format/format";
import { listFormat } from "../format/formatters";
import { type Locale, translatePlural, type useT } from "../i18n";
import { throwProblem } from "../lib/problem";

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
    queryFn: async ({ signal }): Promise<Company | null> => {
      const { data, error, response } = await api.GET("/companies/{id}", {
        signal,
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

// Who holds contacts at a company that this reader may not see, as one
// sentence naming them, or null when nobody does. The employer facts line and
// the company list's Contacts cell both say it. A count of 0 then never reads
// as "no contacts" while a colleague holds some.
export function hiddenHoldersNotice(
  holders: Company["hidden_contact_holders"],
  locale: Locale,
): string | null {
  if (!holders || holders.length === 0) {
    return null;
  }
  const names = listFormat(locale, { type: "conjunction" }).format(
    holders.map((holder) => holder.display_name),
  );
  return translatePlural(
    locale,
    "contact.employer.hiddenHolders",
    holders.length,
    { names },
  );
}

// The counts line under the company name: contacts, then open deals, joined
// the way the rest of this rail joins two short facts on one line. Callers
// gate on `contact_count`/`open_deal_count` being present at all: a company
// the reader lacks the grants to see either number for renders neither.
export function employerFacts(
  company: Pick<
    Company,
    "contact_count" | "open_deal_count" | "hidden_contact_holders"
  >,
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
  const notice = hiddenHoldersNotice(company.hidden_contact_holders, locale);
  if (notice) {
    parts.push(notice);
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

// A company's mark, falling back to its monogram until a logo is on file.
export function CompanyMark({
  companyId,
  name,
  logoUrl,
}: Readonly<{
  companyId: string;
  name: string;
  logoUrl: string | null | undefined;
}>) {
  return (
    <CompanyLogo
      name={name}
      src={logoUrl}
      fallback={<Avatar identity={companyId} name={name} size="sm" />}
    />
  );
}
