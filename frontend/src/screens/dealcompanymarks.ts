import { useQueries } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

// The companies a deals board names, resolved to the names and marks its
// cards draw.

type Deal = components["schemas"]["Deal"];
type Company = components["schemas"]["Company"];

/** A company's display name and the mark drawn beside it. */
export type CompanyMark = { name: string; logoUrl?: string | null };

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
 * The create form's picker reads ONE capped page of companies, and the
 * board took its marks from exactly that page — so a deal whose company fell
 * outside it drew a card with no company row at all, which a reader reads as a
 * deal nobody has linked. The set that has to be resolvable is the set the
 * loaded deals actually name, so the ids that page did not cover are read by
 * id, a hundred to a request: one request per company made a cold board wait
 * on dozens of reads before its cards had names.
 *
 * A withheld company is never among them — the wire sends no id to read — so
 * this cannot turn a mask into a name.
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
    ? [
        ...new Set(
          deals.flatMap((deal) =>
            deal.company_id && !fromPage.has(deal.company_id)
              ? [deal.company_id]
              : [],
          ),
        ),
      ].sort()
    : [];
  const batches: string[][] = [];
  for (let at = 0; at < unnamed.length; at += COMPANY_MARK_BATCH) {
    batches.push(unnamed.slice(at, at + COMPANY_MARK_BATCH));
  }
  const reads = useQueries({
    queries: batches.map((batch) => ({
      queryKey: ["companies", "marks", batch],
      queryFn: async (): Promise<Map<string, CompanyMark>> => {
        const { data, error } = await api.GET("/companies", {
          params: {
            // Archived as well: archiving a company leaves its deals naming
            // it, and the single-record read these replaced answered those.
            query: {
              id: batch,
              include_anchor: true,
              include_archived: true,
              limit: batch.length,
            },
          },
        });
        if (error) {
          // A refused read is not an absence: it is held as an error, so each
          // card it covers says its company did not load rather than drawing
          // none. The same rule the shared reference resolver states
          // (screens/entityref.tsx).
          throwProblem(error);
        }
        // An id missing from the answer is archived, or row scope hides it
        // from this reader, and no retry turns that into a name: the card has
        // no company to draw.
        return new Map(
          data.data.map((company) => [
            company.id,
            { name: company.display_name, logoUrl: company.logo_url },
          ]),
        );
      },
      // A company's name and mark change far more rarely than the board
      // refetches, so a card that already has one does not ask again.
      staleTime: 60_000,
    })),
  });
  const marks = new Map(fromPage);
  const unreadable = new Set<string>();
  reads.forEach((read, index) => {
    const batch = batches[index] ?? [];
    if (read.data) {
      for (const [id, mark] of read.data) {
        marks.set(id, mark);
      }
      return;
    }
    // The error the queryFn deliberately threw rather than settling as an
    // absence. Read here, or the cards it belongs to say "no company" — which
    // is the one thing this read exists to stop them saying.
    if (read.isError) {
      for (const id of batch) {
        unreadable.add(id);
      }
    }
  });
  return { marks, unreadable };
}

/** The most companies one read asks for — the contract's own `id` bound. */
const COMPANY_MARK_BATCH = 100;
