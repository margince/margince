import { RecordTabs } from "../../design-system/recordtabs";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { CONTACT_TABS, type ContactTab } from "../../lib/contacttab";
import { ContactDealsTab } from "./contactdeals";
import { Employers } from "./contactemployers";
import { ContactFilesTab, useContactFiles } from "./contactfiles";
import { ContactMeetingsTab } from "./contactmeetings";
import { ContactNetworkTab } from "./contactnetwork";
import type { Contact360, ContactMomentAction } from "./contactpage";
import { contactTabCounts } from "./contactpage.tabcounts";
import { ContactResearchTab } from "./contactresearch";
import { ContactTimelineTab } from "./contacttabs";

// The tab words every record page shares. A private set for this page is how
// the same tab came to read "Timeline" here and "History" on the account.
export const TAB_LABEL_KEYS: Readonly<Record<ContactTab, MessageKey>> = {
  overview: "tab.overview",
  timeline: "tab.timeline",
  network: "tab.network",
  deals: "tab.deals",
  meetings: "tab.meetings",
  research: "tab.research",
  documents: "tab.documents",
};

/**
 * ContactRecordTabs is the strip over the work column. It carries the tab
 * words every record shares and a count where the page holds the whole list.
 * The dot on Research shows while a connected provider has never been asked
 * about this contact. A cancelled run reads as never run too: nothing was bought.
 *
 * It reads the contact's files for the Documents count through the same query
 * the Documents tab reads, so opening the tab costs no second request.
 */
export function ContactRecordTabs({
  view,
  value,
  onChange,
}: Readonly<{
  view: Contact360;
  value: ContactTab;
  onChange: (next: ContactTab) => void;
}>) {
  const t = useT();
  const files = useContactFiles(view.contact.id);
  const filesRead =
    files.query.isSuccess && !files.hasMore ? files.files.length : undefined;
  return (
    <RecordTabs
      options={CONTACT_TABS}
      value={value}
      onChange={onChange}
      labels={{
        overview: t(TAB_LABEL_KEYS.overview),
        timeline: t(TAB_LABEL_KEYS.timeline),
        network: t(TAB_LABEL_KEYS.network),
        deals: t(TAB_LABEL_KEYS.deals),
        meetings: t(TAB_LABEL_KEYS.meetings),
        research: t(TAB_LABEL_KEYS.research),
        documents: t(TAB_LABEL_KEYS.documents),
      }}
      counts={contactTabCounts(view, filesRead)}
      marks={{
        research: (view.provider_profiles ?? []).some(
          (profile) => profile.state === "never_run",
        ),
      }}
    />
  );
}

/**
 * ContactTabPanel is what the tab bar chose, and nothing else. The page above
 * it decides which record it is on. This decides which face of that record the
 * reader asked for.
 *
 * Four of them read the 360 the page already holds. Documents owns its read
 * because the 360 carries no attachments. The Timeline's changes half owns one
 * for the same reason; its activities half is the 360's own section.
 */
export function ContactTabPanel({
  onOpenEmail,
  tab,
  contactId,
  view,
  onBriefMeeting,
  onAction,
  refusedReasonId,
}: Readonly<{
  tab: ContactTab;
  contactId: string;
  view: Contact360;
  onBriefMeeting: (activityId: string) => void;
  /** Opens one message in the record's drawer, which the page owns. */
  onOpenEmail?: (activityId: string) => void;
  // The page's one action loop, which every moment action opens through. It
  // serves the tab whose moment carries a verb: the Meetings tab's agenda button.
  onAction?: (action: ContactMomentAction) => void;
  // The page's one sentence about why this contact takes no changes, for
  // the tab whose verb writes the record: the upload on Documents.
  refusedReasonId?: string;
}>) {
  switch (tab) {
    case "timeline":
      return (
        <ContactTimelineTab
          onOpenEmail={onOpenEmail}
          contactId={contactId}
          view={view}
          onBriefMeeting={onBriefMeeting}
        />
      );
    case "network":
      return (
        // Where they work, and worked, with the way to add or end a job:
        // the relationship tab's first question.
        <div className="record-stack">
          <Employers view={view} />
          <ContactNetworkTab
            contactId={contactId}
            view={view}
            onOpenEmail={onOpenEmail}
          />
        </div>
      );
    case "deals":
      return <ContactDealsTab view={view} />;
    case "meetings":
      return (
        <ContactMeetingsTab
          view={view}
          onBriefMeeting={onBriefMeeting}
          onAction={onAction}
        />
      );
    case "research":
      return <ContactResearchTab view={view} />;
    case "documents":
      return (
        <ContactFilesTab
          contactId={contactId}
          refusedReasonId={refusedReasonId}
        />
      );
    // Overview is drawn by the page itself, above this component. Its stack
    // reads the page's other queries (the brief), and its moment drives the
    // page's own action loop.
    default:
      return null;
  }
}
