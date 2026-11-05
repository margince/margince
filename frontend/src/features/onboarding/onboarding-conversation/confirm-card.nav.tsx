import { AlertTriangle, Check } from "lucide-react";
import { useEffect, useState } from "react";
import { Badge } from "../../../design-system/atoms";
import { formatNumber } from "../../../format/format";
import { useLocale, type useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import type { CompanyFieldName } from "../onboarding";
import { isWork, type ReviewRow, type RowState } from "./company-review-state";
import {
  CONTACTS_KEY,
  FACTS_KEY,
  type FrozenGroup,
  groupDomId,
} from "./confirm-card.row";
import { jumpToFindings } from "./entries";

// Which section is under the reader's eye, so the nav can mark it current.
// jsdom has no IntersectionObserver, so the hook falls back to the first
// section rather than crashing.
export function useActiveSection(
  groups: readonly FrozenGroup[],
): string | null {
  const [active, setActive] = useState<string | null>(groups[0]?.key ?? null);
  useEffect(() => {
    if (typeof IntersectionObserver === "undefined") {
      return;
    }
    const sections = groups
      .map((group) => document.getElementById(groupDomId(group.key)))
      .filter((node): node is HTMLElement => node !== null);
    if (sections.length === 0) {
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        const topmost = entries
          .filter((entry) => entry.isIntersecting)
          .sort(
            (a, b) => a.boundingClientRect.top - b.boundingClientRect.top,
          )[0];
        if (topmost) {
          setActive(topmost.target.id.replace("ob-triage-group-", ""));
        }
      },
      { rootMargin: "-10% 0px -70% 0px" },
    );
    for (const section of sections) {
      observer.observe(section);
    }
    return () => observer.disconnect();
  }, [groups]);
  return active;
}

// The server 422s confirm on one thing only: a REQUIRED_FIELDS entry left
// empty. `rowFor` folds that into the "required" state (company-review-state.ts),
// so checking the state is checking REQUIRED_FIELDS, with no second predicate.
function isBlocking(state: RowState): boolean {
  return state === "required";
}

// isWork's set minus the blocking one: what the server never gates on
// (optional-empty, weak or middling evidence). Worth a look, never worded or
// coloured as an obstacle.
function isAdvisory(state: RowState): boolean {
  return isWork(state) && state !== "required";
}

// Blocking vs advisory: the split behind the nav's count, its named lists and
// a row's mark. It reads each row's current state every render, never the
// order frozen at mount, so count, lists and mark cannot drift apart.
function outstandingSplit(
  group: FrozenGroup,
  rowByField: ReadonlyMap<CompanyFieldName, ReviewRow>,
): { blocking: readonly ReviewRow[]; advisory: readonly ReviewRow[] } {
  const rows = group.order
    .map((field) => rowByField.get(field))
    .filter((row): row is ReviewRow => row !== undefined && isWork(row.state));
  return {
    blocking: rows.filter((row) => isBlocking(row.state)),
    advisory: rows.filter((row) => isAdvisory(row.state)),
  };
}

// How many of a section's fields the nav names before it falls back to an
// overflow count. Past this, the list grows taller than the section it
// navigates.
const NAV_NAMED_LIMIT = 5;

// A section's badge over its jump button: one number, never two. Any
// outstanding work shows here, so the nav alone tells settled, advisory and
// blocking apart. The blocking count is the solid danger badge (it gates
// confirm). The advisory count is the plain numeral Contacts/Facts use.
function SectionBadge({
  blocking,
  advisory,
  t,
}: Readonly<{
  blocking: readonly ReviewRow[];
  advisory: readonly ReviewRow[];
  t: ReturnType<typeof useT>;
}>) {
  const { locale } = useLocale();
  if (blocking.length === 0 && advisory.length === 0) {
    return (
      <span className="ob-triage-nav-settled">
        <Check aria-hidden />
        <span className="sr-only">{t("ob.conv.triage.sectionSettled")}</span>
      </span>
    );
  }
  if (blocking.length === 0) {
    return (
      <span className="ob-triage-nav-advisory t-caption">
        <b aria-hidden>{formatNumber(advisory.length, locale)}</b>
        <span className="sr-only">
          {t("ob.conv.triage.sectionAdvisory", {
            count: formatNumber(advisory.length, locale),
          })}
        </span>
      </span>
    );
  }
  return (
    <Badge variant="primary" tone="danger">
      <b aria-hidden>{formatNumber(blocking.length, locale)}</b>
      <span className="sr-only">
        {t("ob.conv.triage.sectionBlocking", {
          count: formatNumber(blocking.length, locale),
        })}
      </span>
    </Badge>
  );
}

// The named to-do list under a section, blocking fields first, each a jump
// to its row through the board's shared jump machinery. A warning triangle
// marks a blocking field; an advisory one has no icon, since a hollow ring
// reads as an unticked checkbox. Each row names its tier to a screen reader.
function NavOutstandingList({
  blocking,
  advisory,
  t,
}: Readonly<{
  blocking: readonly ReviewRow[];
  advisory: readonly ReviewRow[];
  t: ReturnType<typeof useT>;
}>) {
  const { locale } = useLocale();
  const rows = [
    ...blocking.map((row) => ({ row, isBlocking: true as const })),
    ...advisory.map((row) => ({ row, isBlocking: false as const })),
  ];
  if (rows.length === 0) {
    return null;
  }
  const named = rows.slice(0, NAV_NAMED_LIMIT);
  const overflow = rows.length - named.length;
  return (
    <ul className="ob-triage-nav-sublist">
      {named.map(({ row, isBlocking }) => (
        <li key={row.field}>
          <button
            type="button"
            className="ob-triage-nav-item"
            data-blocking={isBlocking ? "true" : undefined}
            onClick={() => jumpToFindings([row.field])}
          >
            {isBlocking && <AlertTriangle aria-hidden />}
            <span>{row.label}</span>
            <span className="sr-only">
              {t(
                isBlocking
                  ? "ob.conv.triage.blockingHead"
                  : "ob.conv.triage.advisoryHead",
              )}
            </span>
          </button>
        </li>
      ))}
      {overflow > 0 && (
        <li className="ob-triage-nav-more t-caption">
          {t("ob.conv.triage.sectionMore", {
            count: formatNumber(overflow, locale),
          })}
        </li>
      )}
    </ul>
  );
}

// The count beside Contacts or Facts says "this is what I found", never
// "this needs you". SectionBadge's pill or red would call the crawl an
// obstacle, and nothing there is the human's to resolve. Zero reads as
// settled, so "nothing found" and "nothing built" never look alike.
function SectionQuantity({
  count,
  labelKey,
  t,
}: Readonly<{
  count: number;
  labelKey: MessageKey;
  t: ReturnType<typeof useT>;
}>) {
  const { locale } = useLocale();
  if (count === 0) {
    return (
      <span className="ob-triage-nav-settled">
        <Check aria-hidden />
        <span className="sr-only">{t("ob.conv.triage.sectionSettled")}</span>
      </span>
    );
  }
  return (
    <span className="ob-triage-nav-quantity t-caption">
      <b aria-hidden>{formatNumber(count, locale)}</b>
      <span className="sr-only">
        {t(labelKey, { count: formatNumber(count, locale) })}
      </span>
    </span>
  );
}

// A list of section names, each a jump link naming the fields still open
// under it: a to-do list, not a count to decode. A section with nothing
// outstanding shows a settled mark, so "done" and "not built" never look
// alike.
export function SectionNav({
  groups,
  activeKey,
  rowByField,
  contactsCount,
  factsCount,
  t,
}: Readonly<{
  groups: readonly FrozenGroup[];
  activeKey: string | null;
  rowByField: ReadonlyMap<CompanyFieldName, ReviewRow>;
  /** null when no read backs this proposal. The Contacts section is then
   * absent too (see the `frozen` builder in confirm-card.tsx). */
  contactsCount: number | null;
  factsCount: number;
  t: ReturnType<typeof useT>;
}>) {
  return (
    <nav className="ob-triage-nav" aria-label={t("ob.conv.triage.mapLabel")}>
      <ul className="ob-triage-nav-list">
        {groups.map((group) => {
          const jumpTo = () => {
            const node = document.getElementById(groupDomId(group.key));
            // jsdom has no scrollIntoView; the browser always does.
            node?.scrollIntoView?.({ block: "start", behavior: "smooth" });
          };
          const current = group.key === activeKey ? "true" : undefined;
          if (group.key === CONTACTS_KEY) {
            return (
              <li key={group.key}>
                <button
                  type="button"
                  className="ob-triage-nav-link"
                  aria-current={current}
                  onClick={jumpTo}
                >
                  <span>{t(group.labelKey)}</span>
                  <SectionQuantity
                    count={contactsCount ?? 0}
                    labelKey="ob.conv.triage.contactsCount"
                    t={t}
                  />
                </button>
              </li>
            );
          }
          if (group.key === FACTS_KEY) {
            return (
              <li key={group.key}>
                <button
                  type="button"
                  className="ob-triage-nav-link"
                  aria-current={current}
                  onClick={jumpTo}
                >
                  <span>{t(group.labelKey)}</span>
                  <SectionQuantity
                    count={factsCount}
                    labelKey="ob.conv.triage.factsCount"
                    t={t}
                  />
                </button>
              </li>
            );
          }
          const { blocking, advisory } = outstandingSplit(group, rowByField);
          return (
            <li key={group.key}>
              <button
                type="button"
                className="ob-triage-nav-link"
                aria-current={current}
                onClick={jumpTo}
              >
                <span>{t(group.labelKey)}</span>
                <SectionBadge blocking={blocking} advisory={advisory} t={t} />
              </button>
              <NavOutstandingList
                blocking={blocking}
                advisory={advisory}
                t={t}
              />
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
