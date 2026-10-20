import { useEffect, useRef, useState } from "react";
import { factFieldLabelKey } from "../records/factview";
import { skipReasonText } from "./onboarding";
import type {
  CompanySiteReadFact,
  CompanySiteReadPage,
  Translate,
} from "./onboarding-gate";

// The ticker's own status word, with no URL in it: the path beside it is
// already the URL, spelled once. pageLabel would print the URL a second time,
// in full, next to its own truncated abbreviation.
export function pageStatusWord(
  t: Translate,
  page: CompanySiteReadPage,
): string {
  const reason = reasonOf(t, page);
  if (page.status === "skipped") {
    return t("ob.scan.pageStatusSkipped", { reason });
  }
  if (page.status === "failed") {
    return t("ob.scan.pageStatusFailed", { reason });
  }
  return t("ob.scan.pageStatusFetched");
}

/**
 * The page the ticker should say was "just walked".
 *
 * The wire's `pages` array carries no arrival order. It lists every fetched
 * page before any skipped or failed one, so the last entry is not the newest.
 * After one skip, `.at(-1)` would name that skip forever. What can be known is
 * which URLs are new since the last poll, so only a new arrival replaces the
 * ticker. A poll with no new page keeps what it last showed.
 *
 * One poll can bring a fresh fetch and a fresh skip together, and the skip
 * sorts last only because of the wire order. A fetched page is news and a skip
 * is not, so this hook prefers the latest fetched arrival. It falls back to a
 * skip or failure only when nothing in the batch fetched.
 */
export function useLatestArrivedPage(
  pages: readonly CompanySiteReadPage[],
): CompanySiteReadPage | null {
  const seen = useRef<Set<string>>(new Set());
  const latest = useRef<CompanySiteReadPage | null>(null);
  const arrived = pages.filter((page) => !seen.current.has(page.url));
  const arrivedFetched = arrived.filter((page) => page.status === "fetched");
  const next = arrivedFetched.at(-1) ?? arrived.at(-1) ?? latest.current;
  useEffect(() => {
    for (const page of pages) {
      seen.current.add(page.url);
    }
    latest.current = next;
  });
  return next;
}

// The ticker's status word, reachable in full though it clamps to two lines.
// `aria-label` carries the whole sentence, because browsers disagree on how
// much line-clamped text reaches the accessible name. A button (not `title`,
// which needs a mouse hover) lets a reader expand the clamp on tap or key.
// Toggling flips a CSS class and never the text, so the row's `aria-live`
// fires once per new page and never on expanding an old one.
export function ScanTickerEntry({
  page,
  facts,
  t,
}: Readonly<{
  page: CompanySiteReadPage;
  facts: readonly CompanySiteReadFact[];
  t: Translate;
}>) {
  const [expanded, setExpanded] = useState(false);
  const note = tickerNoteFor(page, facts, t);
  return (
    <li className="ob-scan-ticker-entry" data-page-status={page.status}>
      <span className="ob-scan-ticker-path">{sourcePath(page.url) ?? "/"}</span>
      {/* No button when there is nothing to say yet. For a fetched page with
          no grounded fact, "fetched" describes the request, not what the page
          gave up, so printing it would be filler for a missing finding. */}
      {note !== null && (
        <button
          type="button"
          className="ob-scan-ticker-kind"
          aria-expanded={expanded}
          aria-label={note}
          onClick={() => setExpanded((current) => !current)}
        >
          {note}
        </button>
      )}
    </li>
  );
}

/**
 * What the ticker says the page gave up, or null for "nothing to say yet".
 *
 * A skipped or failed page will never have a fact, so its status word is the
 * note. A fetched page looks for facts whose `evidence_url` names it (the
 * grounding link the deck and record page trust) and takes the most confident.
 * No match means extraction has not reached the page or found nothing on it.
 * That stays null rather than restating the page's own status.
 */
function tickerNoteFor(
  page: CompanySiteReadPage,
  facts: readonly CompanySiteReadFact[],
  t: Translate,
): string | null {
  if (page.status !== "fetched") {
    return pageStatusWord(t, page);
  }
  const found = facts
    .filter((fact) => fact.evidence_url === page.url)
    .sort((a, b) => b.confidence - a.confidence)
    .at(0);
  if (found === undefined) {
    return null;
  }
  return t("ob.scan.tickerFact", {
    field: t(factFieldLabelKey(found.field)),
    value: found.value,
  });
}

// The page a crawled URL is, as the site's own path. Parsed with a pattern
// rather than `new URL`, so a malformed URL yields "no path to show" instead
// of an exception to swallow.
function sourcePath(url: string): string | null {
  const match = /^[a-z][a-z0-9+.-]*:\/\/[^/?#]+(\/[^?#]*)?/i.exec(url);
  const path = match?.[1];
  if (path === undefined || path === "" || path === "/") {
    return null;
  }
  return path.replace(/\/$/, "");
}

export function reasonOf(t: Translate, page: CompanySiteReadPage): string {
  return skipReasonText(t, page.reason);
}
