import type { ReactNode } from "react";
import type { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { coldFieldLabel } from "../../screens/coldfieldlabels";
import type { OpenReceipt } from "../../screens/record360";
import { RelationshipsTab } from "../../screens/relationshiprows";
import type { Company } from "./companies";
import { HierarchyRollupPanel } from "./companies.hierarchy";
import { DeepReadPanel } from "./companydeepread";
import { CompanyProfileForm } from "./companyprofiletab";
import { TechnicalProfilePanel } from "./companytechnical";

// PROFILE_FIELD_LABELS names the profile fields as statements about a company.
//
// Onboarding asks the same fields as questions to us ("What do you sell?"),
// which reads wrong on a prospect's record.
const PROFILE_FIELD_LABELS: Record<string, MessageKey> = {
  display_name: "co.profileField.display_name",
  offer_summary: "co.profileField.offer_summary",
  icp: "co.profileField.icp",
  buying_center: "co.profileField.buying_center",
  value_proposition: "co.profileField.value_proposition",
  usp: "co.profileField.usp",
  customer_pains: "co.profileField.customer_pains",
  desired_outcomes: "co.profileField.desired_outcomes",
  buying_intents: "co.profileField.buying_intents",
  common_objections: "co.profileField.common_objections",
  sales_motion: "co.profileField.sales_motion",
  legal_name: "co.profileField.legal_name",
  registered_address: "co.profileField.registered_address",
  register_vat: "co.profileField.register_vat",
  legal_form: "co.profileField.legal_form",
  register_court: "co.profileField.register_court",
  register_number: "co.profileField.register_number",
  industry: "co.profileField.industry",
  history: "co.profileField.history",
};

// The onboarding wording is the fallback, so a field added there still reads
// as words rather than as a column name here.
export function profileFieldLabel(
  field: string,
  t: ReturnType<typeof useT>,
): string {
  const key = PROFILE_FIELD_LABELS[field];
  return key ? t(key) : coldFieldLabel(field, t);
}

// CompanyProfileTab: the account's own reference material, opened when the
// summary is not enough. It holds the account in its own words and ours, its
// filed fields, its facts, who it is connected to and the one-off tools.
//
// None of it comes from the 360: each card runs its own read. So a failed
// composite read hides what the 360 answered, never the profile, facts or
// relationships.
export function CompanyProfileTab({
  active,
  company,
  // Whether the 360 is already leading with the research offer. It renders in
  // one place. An account with nothing on file meets it at the top of its 360,
  // and every other account finds it here. Both would start the same crawl.
  offerOnOverview,
  onOpenHistory,
  refusedReasonId,
  nameOf,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  readings,
}: Readonly<{
  active: boolean;
  company: Company;
  offerOnOverview: boolean;
  onOpenHistory: () => void;
  // See ReferenceDisclosures: the page's one read-only sentence, by id.
  refusedReasonId?: string;
  // The dossier's own props: the overview's copy and this tab's leading one
  // read the same names and open through the same receipt.
  nameOf?: (entityType: string, entityId: string) => string | undefined;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onOpenReceipt: OpenReceipt;
  onOpenEmail?: (activityId: string) => void;
  // What Margince read into the account beyond its 360, led by the questions.
  readings?: ReactNode;
}>) {
  if (!active) {
    return null;
  }
  return (
    <ReferenceDisclosures
      company={company}
      offerOnOverview={offerOnOverview}
      onOpenHistory={onOpenHistory}
      refusedReasonId={refusedReasonId}
      nameOf={nameOf}
      onOpenRecord={onOpenRecord}
      onOpenReceipt={onOpenReceipt}
      onOpenEmail={onOpenEmail}
      readings={readings}
    />
  );
}

function ReferenceDisclosures({
  company,
  offerOnOverview,
  onOpenHistory,
  refusedReasonId,
  nameOf,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  readings,
}: Readonly<{
  company: Company;
  offerOnOverview: boolean;
  onOpenHistory: () => void;
  // The page's one sentence about why this account takes no changes, while
  // it does not: an edge is written through the account's own write gate.
  refusedReasonId?: string;
  nameOf?: (entityType: string, entityId: string) => string | undefined;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onOpenReceipt: OpenReceipt;
  onOpenEmail?: (activityId: string) => void;
  readings?: ReactNode;
}>): ReactNode {
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
          {readings}
          {/* Each of these names itself, so they stand in the profile's own
              stack, not a titled panel inside a titled panel. Documents have
              their own tab and stay off this one, so a reader never has two
              lists to reconcile. */}
          <RelationshipsTab
            scope={{ company_id: company.id }}
            refusedReasonId={refusedReasonId}
          />

          <HierarchyRollupPanel companyId={company.id} />
          {/* Only where the Brief is not already offering it. An account with
              nothing on file meets the offer at the top of its own column. Two
              offers to research the same company read as none. */}
          {!offerOnOverview && <DeepReadPanel companyId={company.id} />}
          {/* What the company runs, beside what it says. It is read from
              public records the company never wrote for us: DNS, certificates,
              the markup of its homepage. It sits under the site read above,
              because that read is what queues it. */}
          <TechnicalProfilePanel companyId={company.id} />
        </>
      }
    />
  );
}
