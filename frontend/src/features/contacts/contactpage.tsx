import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { PageAsideToggle, usePageAside } from "../../components/pageaside";
import { useHasUnsavedChanges } from "../../components/unsaved";
import { Modal } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { liveProjects } from "../../design-system/projectpicker";
import { RecordTabs } from "../../design-system/recordtabs";
import { RecordView } from "../../design-system/recordview";
import { useT } from "../../i18n";
import { useRecordWriteRefusal } from "../../lib/capability";
import { useOpenEmail } from "../../lib/openemail";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { scrollPageToTop } from "../../lib/reveal";
import { useUrlParams } from "../../lib/urlstate";
import { useFoldedViewport } from "../../lib/viewport";
import { BriefQueue } from "../brief/brief.queue";
import { ContactMeetingBrief } from "../brief/meetingbrief";
import { useWaitingDraftBand } from "../mail/composewaitingdraft";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { contactPageState, contactQuery } from "./contact.queries";
import { useContact360 } from "./contact360";
import { ContactActions } from "./contactactions";
import { ArchivedContactPage } from "./contactarchived";
import { ContactResearchDrawer } from "./contactdrawers";
import { ContactFacts } from "./contactfacts";
import { ContactOpening } from "./contactopening";
import { ContactOverview } from "./contactoverview";
import { BRIEF_PARAM } from "./contactpage.address";
import { ContactRail } from "./contactrail";
import { ContactSubtitle } from "./contactsubtitle";
import { CONTACT_TABS, type ContactTab, contactTabRoute } from "./contacttab";
import "./contact360.css";
import {
  navigateKeepingReturn,
  WorklistReturnLink,
} from "../worklist/worklist.return";
import {
  ContactActivityDrawer,
  ContactMailDrawer,
  ContactWriteTo,
  type Drawer,
  runContactMomentAction,
  useComposer,
} from "./contactpage.drawers";
import { ContactMarks, ContactRoleBadge } from "./contactpage.marks";
import { ContactTabPanel, TAB_LABEL_KEYS } from "./contactpage.tabs";

export { runContactMomentAction } from "./contactpage.drawers";

export type Contact360 = components["schemas"]["Contact360"];

export type ContactMomentAction = components["schemas"]["ContactMomentAction"];

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
  const details = usePageAside();
  // What the pane's switch says in each state: this page's pane holds the
  // permissions as well as the details, and the button names both.
  const paneWords = {
    show: t("contact.overview.detailsShow"),
    hide: t("contact.overview.detailsHide"),
  };
  const narrow = useFoldedViewport();
  const [mobileDetails, setMobileDetails] = useState(false);
  const detailsTitle = useId();
  const detailsDirty = useHasUnsavedChanges("details");
  const closeMobileDetails = () => {
    if (!detailsDirty) setMobileDetails(false);
  };
  const record = useQuery(contactQuery(id));
  // The server refuses the 360, the brief and the guard for an archived row.
  // The 360 runs beside the row read; the other two wait for its answer.
  const live = record.isSuccess && !record.data.archived_at;
  // Read at screen level and handed down, so the schema request runs BESIDE
  // the contact's rather than after it — an edit opened before this landed
  // would otherwise offer a form with no custom fields on it.
  const view = useContact360(id, !record.data?.archived_at);

  const brief = useQuery({
    queryKey: ["contactBrief", id],
    enabled: live,
    queryFn: async () => {
      return unwrap(
        await api.GET("/contacts/{id}/brief", {
          params: { path: { id } },
        }),
      );
    },
  });

  const guard = useQuery({
    queryKey: ["contactConsentGuard", id],
    enabled: live,
    queryFn: async () => {
      return unwrap(
        await api.GET("/contacts/{id}/consent/guard", {
          params: { path: { id } },
        }),
      );
    },
  });

  // Which drawer is open, if any. One at a time: two surfaces over the same
  // record would each claim to be the thing the reader is doing.
  const [drawer, setDrawer] = useState<Drawer>(null);
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
  const band = useWaitingDraftBand({
    anchor: { type: "contact", id },
    composerOpen: drawer === "composer",
    onOpen: () => openComposer(""),
    readOnlyReason,
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

  // The pane is the rail alone. It used to end in an email box with its own
  // "Draft the reply": the same verb the head's Email button already carries,
  // drawn a second time as the pane's one filled control.
  const contactDetails = (
    <ContactRail
      view={view.data}
      guard={guard.data}
      guardLoading={guard.isPending}
      guardFailed={guard.isError}
      onRetryGuard={() => {
        void guard.refetch();
      }}
    />
  );

  return (
    <div className="wrap">
      {/* Inside the gutter, not around it: the page's root is what insets it,
          and everything on the page, the rail included, answers a pressed
          address with this page's own composer. */}
      <ContactWriteTo contactId={id} onWrite={() => openComposer("")}>
        <RecordView
          back={<WorklistReturnLink />}
          // The contact's context, in the details pane beside the work. It is
          // true of the contact, not of the open tab, so a tab change leaves
          // it put. The same pane, fold and memory as every other record. At
          // phone width the same cards open as the drawer below instead.
          aside={narrow ? undefined : contactDetails}
          asideOpen={details.open}
          asideToggle={<PageAsideToggle pane labels={paneWords} />}
          name={contact.full_name}
          identity={contact.id}
          // One rung under the record scale. The name is still the largest
          // thing on the page, but beside the agent's ask it need not be the
          // size of a masthead.
          scale="compact"
          // The seat they hold on the deal beside the name, then what they
          // do, where, and the ways to reach them on one line.
          nameBadge={<ContactRoleBadge role={view.data.commercial?.role} />}
          subtitle={
            <ContactFacts
              contact={contact}
              role={<ContactSubtitle view={view.data} />}
            />
          }
          pulse={<ContactMarks view={view.data} />}
          actions={
            // refusedReasonId is the same sentence the band below states:
            // one write-gate for the whole header, not a narrower one per
            // verb. Edit, merge and archive ride inside the header's menu,
            // so they read it too.
            <ContactActions
              view={view.data}
              contactId={id}
              onWrite={() => openComposer("")}
              onResearch={() => setDrawer("research")}
              onLogActivity={() => setDrawer("activity_log")}
              onAddTask={() => setDrawer("activity_task")}
              refusedReasonId={refusedReasonId}
            />
          }
          actionsInline
          zone={recordZone}
          // Stated once for the page, where both columns and every tab can see
          // it. Every control the record refuses points at its reason by id.
          band={band}
          tabs={
            <RecordTabs
              options={CONTACT_TABS}
              value={tab}
              onChange={(next) => {
                navigateKeepingReturn(contactTabRoute(id, next));
                scrollPageToTop();
              }}
              // On a phone the details open as a drawer, and this is its
              // switch; beside the work the pane carries its own.
              trailing={
                narrow ? (
                  <PageAsideToggle
                    quiet
                    labels={paneWords}
                    controlled={{
                      open: mobileDetails,
                      labels: paneWords,
                      onToggle: () => setMobileDetails((open) => !open),
                    }}
                  />
                ) : undefined
              }
              labels={{
                overview: t(TAB_LABEL_KEYS.overview),
                timeline: t(TAB_LABEL_KEYS.timeline),
                network: t(TAB_LABEL_KEYS.network),
                deals: t(TAB_LABEL_KEYS.deals),
                meetings: t(TAB_LABEL_KEYS.meetings),
                research: t(TAB_LABEL_KEYS.research),
                documents: t(TAB_LABEL_KEYS.documents),
              }}
              // At least one connected provider has never been asked about this
              // contact, so there is a lookup waiting behind the tab. Any of them
              // is enough: the reader still has somebody to ask, even if another
              // provider already answered. A CANCELLED run reads as never_run
              // too, and the dot coming back is right: nothing was bought.
              marks={{
                research: (view.data.provider_profiles ?? []).some(
                  (profile) => profile.state === "never_run",
                ),
              }}
            />
          }
        >
          {tab === "overview" && (
            <ContactOverview
              view={view.data}
              brief={brief.data}
              briefLoading={brief.isLoading}
              briefFailed={brief.isError}
              onRetryBrief={() => {
                brief.refetch();
              }}
              onAction={runAction}
              onOpenEmail={setOpenEmail}
            />
          )}

          <ContactTabPanel
            tab={tab}
            contactId={id}
            view={view.data}
            onBriefMeeting={openBrief}
            onAction={runAction}
            onOpenEmail={setOpenEmail}
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
        {/* Rendered whether or not it is showing, and told so by `open`: a
            drawer whose caller conditions its ELEMENT can only ever appear —
            there is nothing left on the page to animate on its way out. */}
        <Modal
          open={narrow && mobileDetails}
          onClose={closeMobileDetails}
          // The drawer holds fields the reader may be part-way through
          // changing, and `closeMobileDetails` refuses to leave while one is
          // open. The refusal says WHY on the control that carries it, rather
          // than answering the press with nothing.
          closeReason={detailsDirty ? t("record.finishFieldEdit") : undefined}
          labelledBy={detailsTitle}
          intent="drawer"
        >
          <div className="pe-drawer-title">
            <Heading size="large" id={detailsTitle}>
              {t("contact.overview.detailsPermissions")}
            </Heading>
          </div>
          {contactDetails}
        </Modal>
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
