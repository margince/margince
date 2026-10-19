import type { components } from "../../api/schema";
import {
  type CompanyMark,
  useCompanyMarksByID,
} from "../../components/companymarks";

// The companies a deals board names, resolved to the names and marks its
// cards draw.

type Deal = components["schemas"]["Deal"];
type Company = components["schemas"]["Company"];

export type { CompanyMark } from "../../components/companymarks";

/**
 * Every company the loaded deals name, id → mark (`useCompanyMarks` resolves them).
 *
 * A company this reader may not read is in no map: the wire sends
 * `company_id` as null and names it in `masked_fields`, so what the card
 * needs there is the withheld READING, which the card itself spells as the mask
 * — not a name this screen could supply.
 */
export type CompanyMarks = ReadonlyMap<string, CompanyMark>;

/**
 * What the screen knows about the companies its deals name.
 *
 * `unreadable` is the reading the board used to lose. A read that FAILED — a
 * 403 because the reader holds row visibility of the company but no
 * `company:read` grant, a 5xx, a dropped connection — is not the same fact
 * as a deal that names no company, and collapsing the two told the reader the
 * most misleading of the two. The table has always had this reading through
 * `EntityRef`'s failed state; this is the board's half of it.
 */
export type CompanyNaming = Readonly<{
  marks: CompanyMarks;
  unreadable: ReadonlySet<string>;
}>;

/**
 * The company marks the board draws, for every company its cards name.
 *
 * The create form's picker reads one capped page of companies. A company
 * outside that page would leave its card with no company row, so the ids the
 * page did not cover are read by id (`useCompanyMarksByID`). A withheld company
 * sends no id, so this cannot turn a mask into a name.
 */
export function useCompanyMarks(
  deals: Deal[],
  page: Company[],
  pageSettled: boolean,
): CompanyNaming {
  const fromPage = new Map<string, CompanyMark>(
    page.map((company) => [
      company.id,
      { name: company.display_name, logoUrl: company.logo_url },
    ]),
  );
  // Nothing is asked for until the picker's page has ANSWERED. The two reads
  // are issued together and settle in no fixed order, so on every render where
  // the deals have arrived and the companies have not, `fromPage` is empty
  // and every company a loaded deal names looks unresolved.
  const unnamed = pageSettled
    ? deals.flatMap((deal) =>
        deal.company_id && !fromPage.has(deal.company_id)
          ? [deal.company_id]
          : [],
      )
    : [];
  const byID = useCompanyMarksByID(unnamed);
  const marks = new Map(fromPage);
  for (const [id, mark] of byID.marks) {
    marks.set(id, mark);
  }
  return { marks, unreadable: byID.unreadable };
}
