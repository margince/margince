import type { ReactNode } from "react";
import type { OpenReceipt } from "../../components/citations";
import { RecordCustomFields } from "../../components/recordcustomfields";
import { DisclosureGroup } from "../../design-system/disclosure";
import { Panel } from "../../design-system/panel";
import { RelationshipsTab } from "../records/relationshiprows";
import type { Company, Company360View } from "./companies";
import { HierarchyRollupPanel } from "./companies.hierarchy";
import { AssistantPanel } from "./companyassistant";
import { DeepReadPanel } from "./companydeepread";
import { GrowthFitPanel } from "./companygrowthfit";
import { CompanyHoldSection } from "./companyhold";
import { CompanyProfileForm } from "./companyprofiletab";
import { TechnicalProfilePanel } from "./companytechnical";
import { CompanyTriageSection } from "./companytriage";
import { hasWorkInFlight } from "./companywork";

export { profileFieldLabel } from "../../format/companyprofilelabels";

// CompanyProfileTab: the account's own reference material, opened when the
// summary is not enough. It holds the account in its own words and ours, its
// filed fields, its facts, who it is connected to and the one-off tools.
//
// Most cards run their own read, so a failed composite read hides what the
// 360 answered, never the profile, facts or relationships.
export function CompanyProfileTab({
  active,
  company,
  view,
  onOpenHistory,
  refusedReasonId,
  nameOf,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
}: Readonly<{
  active: boolean;
  company: Company;
  // The 360 the page holds: the growth-fit reading asks whether anything is
  // in flight, and the ask scopes a question to one of the projects.
  view?: Company360View;
  onOpenHistory: () => void;
  // The page's one read-only sentence, by id: an edge is written through the
  // account's own write gate.
  refusedReasonId?: string;
  nameOf?: (entityType: string, entityId: string) => string | undefined;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onOpenReceipt: OpenReceipt;
  onOpenEmail?: (activityId: string) => void;
}>): ReactNode {
  if (!active) {
    return null;
  }
  const live = !company.archived_at;
  return (
    <CompanyProfileForm
      company={company}
      onOpenHistory={onOpenHistory}
      nameOf={nameOf}
      onOpenRecord={onOpenRecord}
      onOpenReceipt={onOpenReceipt}
      onOpenEmail={onOpenEmail}
      tools={
        <>
          {/* Is this an account to sell to at all: the question an account
              with nothing in flight is asking. */}
          {live && view && !hasWorkInFlight(view) && (
            <GrowthFitPanel
              companyId={company.id}
              onOpenRecord={onOpenRecord}
              onOpenReceipt={onOpenReceipt}
              onOpenEmail={onOpenEmail}
            />
          )}
          {live && (
            <AssistantPanel
              companyId={company.id}
              onOpenRecord={onOpenRecord}
              onOpenReceipt={onOpenReceipt}
              onOpenEmail={onOpenEmail}
              projects={view?.projects}
            />
          )}
          <RelationshipsTab
            scope={{ company_id: company.id }}
            refusedReasonId={refusedReasonId}
          />
          <HierarchyRollupPanel companyId={company.id} />
          <RecordCustomFields kind="company" record={company} />
          <DeepReadPanel companyId={company.id} />
          {/* What the company runs, read from public records it never wrote
              for us. Under the site read, because that read queues it. */}
          <TechnicalProfilePanel companyId={company.id} />
          {/* Both are about the account's mail domain rather than the work
              on it. */}
          <Panel>
            <DisclosureGroup>
              <CompanyHoldSection company={company} />
              <CompanyTriageSection companyId={company.id} />
            </DisclosureGroup>
          </Panel>
        </>
      }
    />
  );
}
