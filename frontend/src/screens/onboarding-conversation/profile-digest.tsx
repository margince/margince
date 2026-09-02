import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { CompanyFieldName } from "../onboarding";
import type { ReviewRow } from "./company-review-state";
import "./profile-digest.css";

/**
 * The profile as an article, beside the deck that is still writing it.
 *
 * WHY IT SITS HERE AT ALL. The deck asks one thing at a time, which is the
 * right way to ask and a bad way to see: a reader answering the sixth card has
 * no idea what the first five did to the record. This is the record, read as
 * prose, with the line being decided right now marked in it. Answer a card and
 * you watch it land.
 *
 * EVERY LINE CARRIES ITS SOURCE, numbered the way an encyclopedia numbers them:
 * the same page cited twice gets the same number, and the pages themselves are
 * listed once at the foot. That is not decoration. The whole claim of this
 * product is that nothing goes on a record without a page behind it, and a
 * profile that stated its facts without saying where they came from would be
 * asking to be trusted rather than showing its work.
 *
 * A LINE WITH NO SOURCE SAYS SO. Something typed by a person, or carried over
 * from a profile that already existed, has no page to cite and is marked as
 * yours rather than given a number it did not earn.
 */

/** One page the profile cites, and the number it is cited by. */
type Citation = Readonly<{ url: string; n: number }>;

/**
 * Number the pages in the order the profile first cites them.
 *
 * BY URL, so a page backing four fields is one entry cited four times rather
 * than four entries for one page. First-cited order rather than the read's own
 * order, because the numbers are read down the article and a list that jumped
 * would look like pages were missing.
 */
export function citationsOf(rows: readonly ReviewRow[]): Citation[] {
  const seen = new Map<string, number>();
  for (const row of rows) {
    const url = row.evidence?.source;
    if (url === undefined || url === "" || seen.has(url)) {
      continue;
    }
    seen.set(url, seen.size + 1);
  }
  return [...seen].map(([url, n]) => ({ url, n }));
}

/** The path a page is known by here. The host is the same on every row. */
export function pathOf(url: string): string {
  try {
    return new URL(url).pathname || "/";
  } catch {
    return url;
  }
}

export function ProfileDigest({
  rows,
  active,
}: Readonly<{
  rows: readonly ReviewRow[];
  /** The field the deck is asking about, marked in the article as it is decided. */
  active?: CompanyFieldName;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const cites = citationsOf(rows);
  const number = new Map(cites.map((cite) => [cite.url, cite.n]));
  const written = rows.filter((row) => row.value.trim() !== "").length;

  return (
    <aside className="pdigest">
      <p className="pdigest-eyebrow t-eyebrow">{t("ob.digest.where")}</p>
      <p className="pdigest-count">
        {t("ob.digest.written", {
          n: formatNumber(written, locale),
          m: formatNumber(rows.length, locale),
        })}
      </p>
      <div className="pdigest-body">
        {rows.map((row) => (
          <DigestLine
            key={row.field}
            row={row}
            n={
              row.evidence === null
                ? undefined
                : number.get(row.evidence.source)
            }
            active={row.field === active}
          />
        ))}
      </div>
      {cites.length === 0 ? null : (
        <div className="pdigest-sources">
          <p className="pdigest-eyebrow t-eyebrow">{t("ob.digest.sources")}</p>
          <ol>
            {cites.map((cite) => (
              <li key={cite.url}>
                <span className="pdigest-source-n">{cite.n}</span>
                <span className="pdigest-source-path">{pathOf(cite.url)}</span>
              </li>
            ))}
          </ol>
        </div>
      )}
    </aside>
  );
}

// One line of the article: what the record says, and the page that says it.
function DigestLine({
  row,
  n,
  active,
}: Readonly<{ row: ReviewRow; n?: number; active: boolean }>) {
  const t = useT();
  const empty = row.value.trim() === "";
  return (
    <p className="pdigest-line" data-active={active} data-empty={empty}>
      <span className="pdigest-label">{row.label}</span>{" "}
      {empty ? (
        <span className="pdigest-blank">
          {active ? t("ob.digest.deciding") : t("ob.digest.blank")}
        </span>
      ) : (
        <>
          <span className="pdigest-value">{row.value}</span>
          {n === undefined ? (
            // Typed by a person, or carried in from a profile that already
            // existed. It gets no number because there is no page to open, and
            // saying so is the honest half of citing everything else.
            <span className="pdigest-yours">{t("ob.digest.yours")}</span>
          ) : (
            <sup className="pdigest-ref">{n}</sup>
          )}
        </>
      )}
    </p>
  );
}
