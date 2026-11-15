import type { components } from "../../api/schema";
import {
  type CompanyMark,
  type CompanyNaming,
  useCompanyMarksByID,
} from "../../components/companymarks";

// The companies a deals board names, resolved to the names and marks its
// cards draw.

type Deal = components["schemas"]["Deal"];
type Company = components["schemas"]["Company"];

export type {
  CompanyMark,
  CompanyMarks,
  CompanyNaming,
} from "../../components/companymarks";

/**
 * The company marks the board draws, for every company its cards name.
 *
 * The create form's picker reads one capped page of companies. A company
 * outside that page would leave its card with no company row, so the ids the
 * page did not cover are read by id (`useCompanyMarksByID`). A withheld company
 * sends no id, so this cannot turn a mask into a name.
 */
export function useCompanyMarks(deals: Deal[], page: Company[]): CompanyNaming {
  const fromPage = new Map<string, CompanyMark>(
    page.map((company) => [
      company.id,
      { name: company.display_name, logoUrl: company.logo_url },
    ]),
  );
  const unnamed = deals.flatMap((deal) =>
    deal.company_id && !fromPage.has(deal.company_id) ? [deal.company_id] : [],
  );
  const byID = useCompanyMarksByID(unnamed);
  const marks = new Map(fromPage);
  for (const [id, mark] of byID.marks) {
    marks.set(id, mark);
  }
  return { marks, unreadable: byID.unreadable };
}
