import { Panel } from "../../design-system/panel";
import { CompanySuggestions } from "../deals/dealsuggestion";
import { AssistantPanel } from "../voice-knowledge/assistant";
import type { Company360View } from "./companies";
import { LatestSection } from "./company/glance";
import { GrowthFitPanel } from "./companygrowthfit";
import { ContactsSection, SignalsSection } from "./companyrail";
import { DealsSection } from "./companyraildeals";
import { type CompanyOverviewProps, CompanyReading } from "./companyreading";
import { hasWorkInFlight, sinceLastVisitFooter } from "./companywork";
import "./company360.css";

// Whether this account has anything captured against it at all.
//
// All three sections must be empty. A company with one contact and no mail is
// one somebody has started on, and offering to research it would talk over
// their work.
//
// A section the reader may not see is not empty: `sections_omitted` names
// those. Reading a withheld section as "nothing here" would offer a crawl of a
// full account to the one reader who cannot see its contents.
function nothingOnFile(view?: Company360View): boolean {
  if (!view) {
    return false;
  }
  const withheld = new Set(view.sections_omitted ?? []);
  const empty = (
    section: "contacts" | "deals" | "activities",
    rows: number | undefined,
  ) => !withheld.has(section) && rows === 0;
  return (
    empty("contacts", view.contacts?.data.length) &&
    empty("deals", view.deals?.data.length) &&
    empty("activities", view.activities?.data.length)
  );
}

/**
 * Whether the research offer leads the Overview column, rather than sitting on
 * Profile with the record's other tools.
 *
 * Both conditions must hold. First, the account has nothing on file, so its
 * question is "who are these contacts" rather than "what should you do here".
 * Second, it has never been read, because a read already answers that
 * question. A read that filled nothing in is a fact about the site, not an
 * invitation to ask again.
 *
 * Without the second condition, an account with many staged facts would lead
 * with the offer and tell a rep nobody had looked.
 */
export function offerResearchOnOverview({
  view,
  read,
}: Readonly<{
  view?: Company360View;
  /** Whether this account has a website read on record. */
  read: boolean;
}>): boolean {
  return !read && nothingOnFile(view);
}

// The overview's own stack: the 360, then its sections in the order a rep
// works them. It lives apart from CompanyPage so the page stays a layout
// rather than a list of per-tab conditions.
export function CompanyOverviewStack(props: CompanyOverviewProps) {
  const { company, view, loading, onOpenHistory, onOpenTab } = props;
  return (
    <div className="co-overview-stack record-sections">
      {/* The server refuses an archived account's 360 and its lists. */}
      {!company.archived_at && (
        <>
          <CompanyReading {...props} />
          <LatestSection
            key={company.id}
            view={view}
            loading={loading}
            onOpenHistory={onOpenHistory}
            onOpenRecord={props.onOpenRecord}
            onOpenEmail={props.onOpenEmail}
            footer={sinceLastVisitFooter(view)}
          />
          <DealsSection view={view} loading={loading} onTab={onOpenTab} />
          <ContactsSection view={view} loading={loading} onTab={onOpenTab} />
        </>
      )}
    </div>
  );
}

/**
 * CompanyReadings is what Margince read into the account beyond its 360. It
 * holds the prepared questions, the growth fit, a suggested deal and the
 * signals. They stand on Profile, so the overview keeps to the account's
 * own records.
 */
export function CompanyReadings({
  company,
  view,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
}: Readonly<
  Pick<
    CompanyOverviewProps,
    "company" | "view" | "onOpenRecord" | "onOpenReceipt" | "onOpenEmail"
  >
>) {
  return (
    <>
      <div className="co-glance-ask">
        <AssistantPanel
          companyId={company.id}
          onOpenRecord={onOpenRecord}
          onOpenReceipt={onOpenReceipt}
          onOpenEmail={onOpenEmail}
          projects={view?.projects}
        />
      </div>
      {/* Is this an account we should be selling to at all: the question an
          account with nothing in flight is asking. */}
      {!hasWorkInFlight(view) && (
        <GrowthFitPanel
          companyId={company.id}
          onOpenRecord={onOpenRecord}
          onOpenReceipt={onOpenReceipt}
          onOpenEmail={onOpenEmail}
        />
      )}
      {/* A deal the evidence says this account should have, beside the
          signals it was partly read from. */}
      <CompanySuggestions companyId={company.id} />
      <Panel className="co-signals">
        <SignalsSection companyId={company.id} />
      </Panel>
    </>
  );
}
