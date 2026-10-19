import { Panel } from "../../design-system/panel";
import { AssistantPanel } from "../../screens/assistant";
import { CompanySuggestions } from "../deals/dealsuggestion";
import type { Company360View } from "./companies";
import { recordNamesIn } from "./company360";
import { DossierPanel } from "./companydossier";
import { GrowthFitPanel } from "./companygrowthfit";
import { SignalsSection } from "./companyrail";
import { type CompanyOverviewProps, CompanyReading } from "./companyreading";
import { hasWorkInFlight } from "./companywork";

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

// The overview's own stack: one column of same-shaped panels, in the order a
// rep works them. It lives apart from CompanyPage so the page stays a layout
// rather than a list of per-tab conditions.
export function CompanyOverviewStack(props: CompanyOverviewProps) {
  const { company, view, onOpenRecord, onOpenReceipt, onOpenEmail } = props;
  const records = recordNamesIn(view);
  return (
    <div className="co-overview-stack record-sections">
      {!company.archived_at && <CompanyReading {...props} />}
      {/* The account in prose, beside the reading of it. The 360 answers what
          to do. This answers what the account is, in sentences with their
          sources under them. */}
      <DossierPanel
        companyId={company.id}
        nameOf={records}
        onOpenRecord={onOpenRecord}
        onOpenReceipt={onOpenReceipt}
        onOpenEmail={onOpenEmail}
      />
      {/* The prepared questions are the ones the 360 answers in prose. Both
          are written server-side from this reader's own 360, and both cite
          records through the same receipt. They sit beside the reading, not
          at the foot of the page, so a reader can ask at the top. */}
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
          account with nothing in flight is asking. Its own card,
          carrying its own attribution and its own reassess verb in a footer
          band. */}
      {!hasWorkInFlight(view) && (
        <GrowthFitPanel
          companyId={company.id}
          onOpenRecord={onOpenRecord}
          onOpenReceipt={onOpenReceipt}
          onOpenEmail={onOpenEmail}
        />
      )}
      {/* What Margince noticed on this account that nobody asked it to look
          for: promises made, blockers named, risks read out of meetings,
          mail and invoices. */}
      {/* A deal the evidence says this account should have, when Deal Scout
          found one: beside the signals it was partly read from. */}
      <CompanySuggestions companyId={company.id} />
      <Panel className="co-signals">
        <SignalsSection companyId={company.id} />
      </Panel>
    </div>
  );
}
