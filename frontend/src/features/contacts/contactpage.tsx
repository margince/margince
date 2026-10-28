import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useCallback, useId, useState } from "react";
import type { components } from "../../api/schema";
import {
  LatestInboundLine,
  useLatestInbound,
} from "../../components/latestinbound";
import { liveProjects } from "../../design-system/projectpicker";
import { RecordView } from "../../design-system/recordview";
import { useT } from "../../i18n";
import { useRecordWriteRefusal } from "../../lib/capability";
import { contactPageState, contactQuery } from "../../lib/contact.queries";
import { BRIEF_PARAM } from "../../lib/contactpage.address";
import { type ContactTab, contactTabRoute } from "../../lib/contacttab";
import { useOpenEmail } from "../../lib/openemail";
import { useRecordZone } from "../../lib/recordzone";
import { scrollPageToTop } from "../../lib/reveal";
import { useUrlParams } from "../../lib/urlstate";
import { BriefQueue } from "../brief/brief.queue";
import { ContactMeetingBrief } from "../brief/meetingbrief";
import { useWaitingDraftBand } from "../mail/composewaitingdraft";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { useContact360 } from "./contact360";
import { ContactActions, useActivityGate } from "./contactactions";
import { ArchivedContactPage } from "./contactarchived";
import { useConsentGuard } from "./contactconsentpanel";
import { ContactResearchDrawer } from "./contactdrawers";
import { ContactOpening } from "./contactopening";
import { ContactOverview } from "./contactoverview";
import { ContactRail } from "./contactrail";
import { ContactSubtitle } from "./contactsubtitle";
import { ContactLatestInbound } from "./contacttabs";
import "./contact360.css";
import {
  navigateKeepingReturn,
  WorklistReturnLink,
} from "../worklist/worklist.return";
import {
  ContactActivityDrawer,
  ContactCallStart,
  ContactMailDrawer,
  ContactWriteTo,
  type Drawer,
  runContactMomentAction,
  useComposer,
} from "./contactpage.drawers";
import { ContactMarks } from "./contactpage.marks";
import { ContactRecordTabs, ContactTabPanel } from "./contactpage.tabs";

export { runContactMomentAction } from "./contactpage.drawers";

export type Contact360 = components["schemas"]["Contact360"];

export type ContactMomentAction = components["schemas"]["ContactMomentAction"];

// The refusals the left card's footnote states, or nothing while the record
// and the activity gate refuse nothing.
function refusalsOf(
  readOnlyReason: string | undefined,
  readOnlyReasonId: string,
  activity: ReturnType<typeof useActivityGate>,
  t: ReturnType<typeof useT>,
): ReactNode {
  const activityRefused = activity.refusedId !== undefined;
  if (!readOnlyReason && !activityRefused) {
    return undefined;
  }
  return (
    <>
      {readOnlyReason && <span id={readOnlyReasonId}>{readOnlyReason}</span>}
      {readOnlyReason && activityRefused && " "}
      {activityRefused && (
        <span id={activity.sentenceId}>{t("record.logActivityRefused")}</span>
      )}
    </>
  );
}

export function ContactPageV2({
  id,
  tab,
}: Readonly<{ id: string; tab: ContactTab }>) {
  // Which message the drawer is showing. One drawer over the record, owned by
  // the page: the timeline and the rail both open into it, so a citation in
  // the aside and a row in the body lead to the same place.
  const [openEmail, setOpenEmail] = useOpenEmail();
  const t = useT();
  const recordZone = useRecordZone();
  const record = useQuery(contactQuery(id));
  // The server refuses the 360 and the guard for an archived row. The 360
  // runs beside the row read; the guard waits for its answer.
  const live = record.isSuccess && !record.data.archived_at;
  // Read at screen level and handed down, so the schema request runs BESIDE
  // the contact's rather than after it — an edit opened before this landed
  // would otherwise offer a form with no custom fields on it.
  const view = useContact360(id, !record.data?.archived_at);

  const guard = useConsentGuard(id, live);
  const activity = useActivityGate();
  const latest = useLatestInbound("contact", id);
  const writeRefusalId = useId();

  // Which drawer is open, if any. One at a time: two surfaces over the same
  // record would each claim to be the thing the reader is doing.
  const [drawer, setDrawer] = useState<Drawer>(null);
  const openCallLog = useCallback(() => setDrawer("activity_call"), []);
  const [briefedMeeting, openBrief] = useBriefedMeeting();
  // What the action that opened the composer wanted written. A rung knows WHY
  // it fired and says so in its prefill; before this the client dropped that
  // on the floor and opened the same empty composer as the generic button, so
  // "Draft a follow-up" and "Write an email" did exactly the same thing.
  const [composerIntent, setComposerIntent] = useState("");
  // BOTH doors to the composer, answered in one place: the address opens it on
  // arrival, pressing "Write an email" opens it after. The page is at its
  // complexity ceiling — the lint says so — so the question is settled in the
  // hook rather than in two conditionals in the markup.
  const composer = useComposer(t, drawer === "composer", composerIntent, () =>
    setDrawer(null),
  );

  // Every path to the composer goes through here, and the intent it opens with
  // is decided ONCE, on the way in. Two rules the two callers each get wrong on
  // their own: the generic "Write an email" must clear the reason the last rung
  // left behind, or it inherits a subject nobody asked for; and a composer
  // already on screen keeps what it holds, because a reader mid-sentence is not
  // asking to have their words replaced.
  const openComposer = (intent: string) => {
    if (drawer !== "composer") {
      setComposerIntent(intent);
    }
    setDrawer("composer");
  };

  // Every write affordance on this page that changes the RECORD answers one
  // question, asked once: an archived contact takes no changes, and one this
  // caller cannot write takes none from them. The rail's inline fields ask
  // the same of the same row (contactrail.tsx); this is the header's and the
  // tabs' copy of the answer, and the one sentence both point at.
  const readOnlyReasonId = useId();
  // A 360 cached from before the archive would not say the row is archived.
  const shownRow = record.data?.archived_at ? record.data : view.data?.contact;
  const readOnlyReason = useRecordWriteRefusal("contact", shownRow, {
    archived: t("contact.rail.archivedReadOnly"),
    notYours: t("contact.notYoursToChange"),
  });
  const refusedReasonId = readOnlyReason ? readOnlyReasonId : undefined;
  // A live contact states its read-only sentence in the left card's
  // footnote; the archived page has no such card, so its band states it.
  const band = useWaitingDraftBand({
    anchor: { type: "contact", id },
    composerOpen: drawer === "composer",
    onOpen: () => openComposer(""),
    readOnlyReason: record.data?.archived_at ? readOnlyReason : undefined,
    readOnlyReasonId,
  });

  if (record.data?.archived_at) {
    return (
      <ArchivedContactPage
        contact={record.data}
        band={band}
        refusedReasonId={readOnlyReasonId}
      />
    );
  }
  const reads = contactPageState(record, view);
  if (reads === "opening") return <ContactOpening id={id} />;
  if (reads === "notOpened" || !view.data) {
    return <div className="wrap">{t("contact.page.notOpened")}</div>;
  }

  const contact = view.data.contact;
  // The guard is the RAIL'S, not the hero button's. It answers one verdict per
  // purpose, and the button used to refuse when none of them was `allowed` —
  // which asked a question this page cannot answer, because whether a message
  // may go depends on what the message IS. A contact who stopped the newsletter
  // can still be sent their invoice, and telling their rep the product would
  // not let them write at all is how a lawful message goes out some other way.

  // The action loop. Every surface the contract can name routes to
  // `runContactMomentAction`, a standalone function rather than a closure here
  // so its switch's own branches are not this component's cognitive weight.
  const runAction = (action: ContactMomentAction) =>
    runContactMomentAction(action, t, {
      contactId: id,
      openComposer,
      setDrawer,
      openBrief,
      nextMeetingId: view.data.next_meeting?.activity_id ?? null,
    });

  // What the record refuses this reader, stated once at the foot of the left
  // card: every refused verb points at its sentence by id. The rail adds the
  // Email verb's own refusal, which it reads from the same transports.
  const refusals = refusalsOf(readOnlyReason, readOnlyReasonId, activity, t);
  const contactDetails = (
    <ContactRail
      view={view.data}
      consent={{
        guard: guard.data,
        loading: guard.isPending,
        failed: guard.isError,
        onRetry: () => {
          void guard.refetch();
        },
      }}
      refusals={refusals}
      writeRefusalId={writeRefusalId}
    />
  );

  const openTab = (next: ContactTab) => {
    navigateKeepingReturn(contactTabRoute(id, next));
    scrollPageToTop();
  };

  return (
    <div className="wrap">
      {/* Inside the gutter, not around it: the page's root is what insets it,
          and everything on the page, the rail included, answers a pressed
          address with this page's own composer. */}
      <ContactWriteTo contactId={id} onWrite={() => openComposer("")}>
        <ContactCallStart
          contactId={id}
          canLog={!activity.pending && activity.refusedId === undefined}
          onCall={openCallLog}
        >
          <RecordView
            back={<WorklistReturnLink />}
            // The contact's details, under the head in the left card. They hold
            // for the contact, not the open tab, so a tab change leaves them put.
            // The split folds them above the feed once the row has no room.
            aside={contactDetails}
            name={contact.full_name}
            identity={contact.id}
            // What they do and where under the name, then the pills. The ways
            // to reach them are details, and live in the aside.
            nameBadge={<ContactSubtitle view={view.data} />}
            pulse={<ContactMarks view={view.data} />}
            latest={
              tab !== "overview" && (
                <LatestInboundLine
                  latest={latest}
                  onOpen={() => openTab("overview")}
                />
              )
            }
            actions={
              // refusedReasonId is the same sentence the band below states.
              // It is one write gate for the whole header, not a narrower one per
              // verb. Edit, merge and archive ride inside the header's menu,
              // so they read it too.
              <ContactActions
                view={view.data}
                contactId={id}
                onWrite={() => openComposer("")}
                onResearch={() => setDrawer("research")}
                onLogActivity={() => setDrawer("activity_log")}
                onAddTask={() => setDrawer("activity_task")}
                activity={activity}
                writeRefusalId={writeRefusalId}
                refusedReasonId={refusedReasonId}
              />
            }
            zone={recordZone}
            // Stated once for the page, where both columns and every tab can see
            // it. Every control the record refuses points at its reason by id.
            band={band}
            tabs={
              <ContactRecordTabs
                view={view.data}
                value={tab}
                onChange={openTab}
              />
            }
          >
            {tab === "overview" && (
              <>
                <ContactLatestInbound
                  latest={latest}
                  contactId={id}
                  view={view.data}
                />
                <ContactOverview
                  view={view.data}
                  onAction={runAction}
                  onOpenEmail={setOpenEmail}
                  onOpenTab={openTab}
                />
              </>
            )}

            <ContactTabPanel
              tab={tab}
              contactId={id}
              view={view.data}
              onBriefMeeting={openBrief}
              onAction={runAction}
              onOpenEmail={setOpenEmail}
              onWrite={() => openComposer("")}
              refusedReasonId={refusedReasonId}
            />
            {/* One drawer over the record. The timeline's rows and the rail's
          citations both open into it, so a reader who finds a message in the
          aside and one who finds it in the body land in the same place. */}
            <OpenEmailDrawer
              activityId={openEmail}
              zone={recordZone}
              onClose={() => setOpenEmail(null)}
            />
            <ContactMailDrawer
              contactId={id}
              view={view.data}
              recordAddress={contact.primary_email ?? undefined}
              open={composer.open}
              intent={composer.intent}
              threadId={composer.threadId}
              draftOnOpen={composer.draftOnOpen}
              onClose={composer.close}
            />
            <ContactResearchDrawer
              contactId={id}
              contactName={contact.full_name}
              providerProfiles={view.data.provider_profiles}
              open={drawer === "research"}
              onClose={() => setDrawer(null)}
            />
            <ContactMeetingBrief
              activityId={briefedMeeting}
              open={briefedMeeting !== null}
              onClose={() => openBrief(null)}
              projects={liveProjects(view.data.projects)}
              onOpenEmail={setOpenEmail}
            />
            <ContactActivityDrawer
              contactId={id}
              drawer={drawer}
              onClose={() => setDrawer(null)}
            />
            {/* The work queue, over the record: the same drawer the Home page
            opens, driven by the same address, so a link to it from here
            and from there is one link. */}
            <BriefQueue />
          </RecordView>
        </ContactCallStart>
      </ContactWriteTo>
    </div>
  );
}

/**
 * Which meeting the address says to brief, and how to change it.
 *
 * The meeting brief is the one drawer on this page with an address, because
 * another screen sends a reader to it. The other three open by pressing
 * something here, and need no name.
 *
 * Derived from the query rather than seeded from it, and the difference shows
 * on Back. A drawer held in useState stays open when the reader navigates back
 * out of it, since nothing tells it the address changed. Deriving makes Back
 * close it, which is what a reader pressing Back means.
 *
 * A hook rather than four lines in the page: the page is at its complexity
 * ceiling (the lint says so). This is one idea a reader takes in on its own.
 */
function useBriefedMeeting(): [
  string | null,
  (activityId: string | null) => void,
] {
  const [params, setParams] = useUrlParams();
  const setBriefed = (activityId: string | null) => {
    const out = new Map(params);
    if (activityId) {
      out.set(BRIEF_PARAM, activityId);
    } else {
      out.delete(BRIEF_PARAM);
    }
    setParams(out);
  };
  return [params.get(BRIEF_PARAM) ?? null, setBriefed];
}
