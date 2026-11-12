import { Link } from "lucide-react";
import { type Ref, useRef, useState } from "react";
import type { components } from "../../api/schema";
import { useActivityGate } from "../../components/activitygate";
import { RecordMeetings } from "../../components/recordmeetings";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { useCopyBookingLink } from "../../lib/bookinglinkcopy";
import { useCan } from "../../lib/capability";
import { navigate } from "../../lib/router";
import { useSchedulingProfile } from "../../lib/scheduling-profile-query";
import {
  type CopiedLink,
  useMeetingProposals,
  WaitingSection,
} from "./contactmeetings.waiting";
import { momentDrafts } from "./contactpage.drawers";
import "./contact360.css";

type Contact360 = components["schemas"]["Contact360"];
type ContactMomentAction = components["schemas"]["ContactMomentAction"];

// One verb off a moment, rendered where the meeting it is about already sits.
// The moment's OWN card (contacttoday.tsx) carries the same contract for the
// record's front page; this is the tab's copy of the button, not of the
// gating logic: `state` and `blocked_reason` come straight off the action.
function MeetingMomentAction({
  action,
  onAction,
}: Readonly<{
  action: ContactMomentAction;
  onAction?: (action: ContactMomentAction) => void;
}>) {
  const t = useT();
  if (!onAction) {
    return null;
  }
  const blocked = action.state === "blocked";
  const drafts = momentDrafts(action);
  return (
    <Button
      variant={drafts ? "aiQuiet" : "ghost"}
      onClick={() => onAction(action)}
      reason={
        blocked
          ? (action.blocked_reason ?? t("contact.rail.blocked"))
          : undefined
      }
    >
      {action.label}
    </Button>
  );
}

/**
 * ContactMeetingsTab is the record's Meetings tab with the contact's own
 * parts: the ways to book them, and the invitations still waiting on their
 * answer. The booked meeting leads from this contact's activity link, not
 * their account's: the company's answer names a meeting this contact may not
 * be in.
 */
export function ContactMeetingsTab({
  view,
  loading = false,
  onBriefMeeting,
  onAction,
}: Readonly<{
  view?: Contact360;
  loading?: boolean;
  onBriefMeeting?: (activityId: string) => void;
  // Runs a moment's own verb ("Draft agenda" today, whatever the ladder grows
  // next tomorrow) through the page's one action loop, the same door the
  // record's front page opens.
  onAction?: (action: ContactMomentAction) => void;
}>) {
  const canReadActivity = useCan("activity", "read");
  // Reading past proposals does not attach anything to the contact.
  const proposals = useMeetingProposals(
    canReadActivity ? view?.contact.id : undefined,
  );
  const bookButton = useRef<HTMLButtonElement>(null);
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);
  const copied: CopiedLink = { url: copiedUrl, onCopied: setCopiedUrl };
  // The prep chip and its agenda verb belong only to a meeting-prep moment,
  // which is about the next meeting. Other moments say nothing about one.
  const meetingPrep =
    view?.moment?.rule === "meeting_prep" ? view.moment : undefined;
  return (
    <RecordMeetings
      entity="contact"
      recordId={view?.contact.id}
      view={view}
      loading={loading}
      next={view?.next_meeting}
      onBriefMeeting={onBriefMeeting}
      toolbar={
        <MeetingsToolbar
          view={view}
          loading={loading}
          copied={copied}
          bookButton={bookButton}
        />
      }
      nextNote={meetingPrep && { by: "Margince", whyNow: meetingPrep.why_now }}
      nextVerbs={meetingPrep?.secondary_actions?.map((action) => (
        <MeetingMomentAction
          key={action.label}
          action={action}
          onAction={onAction}
        />
      ))}
      waiting={
        view && (
          <WaitingSection
            contact={view.contact}
            proposals={proposals}
            copied={copied}
            afterWithdraw={() => bookButton.current}
          />
        )
      }
    />
  );
}

// The two ways to start a meeting: book one, or hand the contact the reader's
// own booking link.
function MeetingsToolbar({
  view,
  loading,
  copied,
  bookButton,
}: Readonly<{
  view?: Contact360;
  loading: boolean;
  copied: CopiedLink;
  bookButton: Ref<HTMLButtonElement>;
}>) {
  const t = useT();
  const bookingLink = useBookingLink(copied);
  return (
    <div className="pe-meetings-toolbar">
      <div className="pe-meetings-verbs">
        <BookMeeting view={view} loading={loading} ref={bookButton} />
        {bookingLink.url && (
          <Button onClick={bookingLink.copy.copy}>
            <Link aria-hidden="true" />
            {t(
              copied.url === bookingLink.url
                ? "scheduling.copied"
                : "contact.meetings.copyBookingLink",
            )}
          </Button>
        )}
      </div>
      {bookingLink.copy.notice && (
        <div className="pe-copy-notice">
          {bookingLink.copy.notice}
          <p className="pe-copy-url">{bookingLink.url}</p>
        </div>
      )}
    </div>
  );
}

// The tab's own verb: a meeting with this contact, from the booking flow.
function BookMeeting({
  view,
  loading,
  ref,
}: Readonly<{
  view?: Contact360;
  loading: boolean;
  ref: Ref<HTMLButtonElement>;
}>) {
  const t = useT();
  const gate = useActivityGate(view?.contact ?? null, "scheduling.bookRefused");
  return (
    <Button
      ref={ref}
      variant="primary"
      disabled={loading || !view || gate.pending}
      reason={gate.refusalKey ? t(gate.refusalKey) : undefined}
      onClick={() => {
        if (view)
          navigate({ screen: "book", id: `contact-${view.contact.id}` });
      }}
    >
      {t("scheduling.bookContact")}
    </Button>
  );
}

// The reader's own public link, for a contact who would rather pick a time
// themselves. Offered only while the page takes bookings: a paused link hands
// the contact a page that turns them away.
function useBookingLink(copied: CopiedLink) {
  const t = useT();
  const profile = useSchedulingProfile();
  const url = profile.data?.enabled ? (profile.data.public_url ?? "") : "";
  const copy = useCopyBookingLink(
    url,
    {
      copy: t("contact.meetings.copyBookingLink"),
      copied: t("scheduling.copied"),
      remedy: t("scheduling.copyFallback"),
    },
    () => copied.onCopied(url),
  );
  return { url, copy };
}
