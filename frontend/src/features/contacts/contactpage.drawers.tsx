import type { ReactNode } from "react";
import { useCallback, useRef } from "react";
import { LogActivityAction } from "../../components/logactivity.action";
import type { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  type CallStart,
  CallStartProvider,
  useCallStart,
} from "../../lib/callstart";
import { intentAbout } from "../../lib/compose.intent";
import { COMPOSE_PARAM, THREAD_PARAM } from "../../lib/contactpage.address";
import {
  transportForActivity,
  useTransports,
} from "../../lib/contacttransports";
import { navigate } from "../../lib/router";
import { useUrlParams } from "../../lib/urlstate";
import { useWriteTo, type WriteTo, WriteToProvider } from "../../lib/writeto";
import { ComposeModal } from "../mail/compose";
import { useMailboxConnected } from "../settings/writetohost";
import { navigateKeepingReturn } from "../worklist/worklist.return";
import type { Contact360, ContactMomentAction } from "./contactpage";

// The intent phrases a moment action can ask the composer to open with. The
// server names the reason in its own vocabulary ("agenda", "follow_up"). The
// composer hands it to a language model and shows it to a rep: both need a sentence.
const COMPOSER_INTENT_KEYS: Readonly<Record<string, MessageKey>> = {
  agenda: "contact.composer.intentAgenda",
  reply: "contact.composer.intentReply",
  deliver_commitment: "contact.composer.intentCommitment",
  follow_up: "contact.composer.intentFollowUp",
};

// What the composer opens with, from an action's prefill.
//
// An intent this client does not know opens an empty composer; the raw token
// is never passed through. "deliver_commitment" in the intent field is worse
// than nothing. A rep reads it as something the product meant to say, and the
// model reads it as an instruction nobody wrote.
function composerIntentOf(
  prefill: Readonly<Record<string, string>> | undefined,
  t: ReturnType<typeof useT>,
): string {
  const key = COMPOSER_INTENT_KEYS[prefill?.intent ?? ""];
  if (!key) {
    return "";
  }
  // The promise itself rides in `subject`. Without it, a rung firing on one of
  // several open commitments leaves the drafter to guess which was promised.
  // Joined by the composer's own helper: the queue's prepared reply seeds the
  // same field. A phrase joined two ways is two answers to what the model is told.
  return intentAbout(t(key), prefill?.subject);
}

/**
 * Whether pressing this action has the model write. One rule for the press and
 * for the face: a verb that drafts wears indigo, and one that wears indigo drafts.
 */
export function momentDrafts(action: ContactMomentAction): boolean {
  return action.destination
    ? action.destination.surface === "composer"
    : action.kind === "draft_reply";
}

// What a moment card's action does, off the surface the server named. A
// standalone function rather than a closure inside ContactPageV2: the switch's
// branches are this function's cognitive weight, not the page's.
//
// A destination this client cannot route opens nothing rather than guessing.
// The typed descriptor exists so a button with no path is never rendered.
// Silently doing something else would be worse than the 404 it prevents.
export function runContactMomentAction(
  action: ContactMomentAction,
  t: ReturnType<typeof useT>,
  handlers: Readonly<{
    contactId: string;
    openComposer: (intent: string, drafts?: boolean) => void;
    setDrawer: (drawer: Drawer) => void;
    openBrief: (activityId: string | null) => void;
    // The prep moment is raised off the next meeting, so that is the one it
    // briefs by default. An action naming its own activity is honoured over
    // it, because a moment about a different meeting must not open a brief
    // about this one.
    nextMeetingId: string | null;
  }>,
): void {
  const destination = action.destination;
  if (!destination) {
    // An action with no destination is its own destination. The composer is
    // the sensible home for the drafting kinds.
    if (momentDrafts(action)) {
      handlers.openComposer("", true);
    }
    return;
  }
  switch (destination.surface) {
    case "booking":
      navigate({ screen: "book", id: `contact-${handlers.contactId}` });
      return;
    case "composer":
      handlers.openComposer(composerIntentOf(destination.prefill, t), true);
      return;
    case "research":
      handlers.setDrawer("research");
      return;
    case "meeting_brief":
      handlers.openBrief(destination.entity_id ?? handlers.nextMeetingId);
      return;
    case "record":
      if (destination.entity_id) {
        navigateKeepingReturn({ screen: "deals", id: destination.entity_id });
      }
      return;
    case "activity_log":
      handlers.setDrawer("activity_log");
      return;
    case "task":
      handlers.setDrawer("activity_task");
      return;
    default:
      // Every surface ContactMomentDestination documents today is handled
      // above, so the compiler holds this exhaustive. A surface the contract
      // adds and this switch misses is a type error here, never a dead button.
      destination.surface satisfies never;
      return;
  }
}

/**
 * This page's answer to an address pressed on it: its own composer.
 *
 * The shell hosts a composer for an address pressed anywhere. This page keeps
 * one already: the drawer that also offers the contact's chat transports.
 * Opening the shell's beside it would bring back the two composers
 * ContactMailDrawer replaced. The drawer leads with mail when the contact has
 * an address. An address of some other record still goes to the shell.
 *
 * Under the shell's own condition: a reader with no connected mailbox gets no
 * answer from this page either. The address hands itself to their own mail
 * client, as it would anywhere else.
 */
export function ContactWriteTo({
  contactId,
  onWrite,
  children,
}: Readonly<{
  contactId: string;
  onWrite: (draftIntent: string | undefined) => void;
  children: ReactNode;
}>) {
  const shell = useWriteTo();
  const connected = useMailboxConnected();
  const writeTo: WriteTo = (target) => {
    if (target.entityType === "contact" && target.entityId === contactId) {
      onWrite(target.draftIntent);
      return;
    }
    shell?.(target);
  };
  return (
    <WriteToProvider writeTo={connected ? writeTo : null}>
      {children}
    </WriteToProvider>
  );
}

// A call to this contact opens the page's own Log activity drawer, so the page
// never shows two log forms. A call to anyone else goes on to the shell.
export function ContactCallStart({
  contactId,
  canLog,
  onCall,
  children,
}: Readonly<{
  contactId: string;
  // The page's own activity gate, so the number refuses what its Log verb does.
  canLog: boolean;
  onCall: () => void;
  children: ReactNode;
}>) {
  const shell = useCallStart();
  // Stable, so the provider does not re-render every number on the page.
  const start = useCallback<CallStart>(
    (record) => {
      if (record.entityId === contactId) {
        onCall();
        return;
      }
      shell?.(record);
    },
    [contactId, onCall, shell],
  );
  return (
    <CallStartProvider start={canLog ? start : null}>
      {children}
    </CallStartProvider>
  );
}

// Which drawer is open. Null is the ordinary state: the page is what the
// reader is looking at, and a drawer is a detour from it.
//
// The meeting brief names the meeting it is briefing. A bare "meeting" reading
// `next_meeting` would reach only the soonest meeting, and only while the prep
// moment is live. Every other meeting has a brief the backend can assemble.
export type Drawer =
  | "composer"
  | "research"
  | "activity_log"
  | "activity_task"
  | "activity_call"
  | null;

/**
 * Whether the composer is open, what it is about, and how to close it.
 *
 * Two doors, one answer. A reader presses "Write an email" on this page, or
 * arrives on `?compose=reply` from another screen (the worklist's `draft_reply`
 * move). Both end here, so neither caller has to know about the other.
 *
 * The address half is derived, for the same reason as the brief drawer. A
 * drawer held in state stays open when the reader navigates back out of it.
 * Deriving makes Back close it, which is what pressing Back means.
 *
 * An intent this client does not know opens nothing, not even an empty
 * composer. An address is something a contact can type. A stray `?compose=x`
 * silently starting an email is worse than a link that does not work.
 */
/** What a press on this page asked the composer for. */
export type PressedComposer = Readonly<{ intent: string; drafts: boolean }>;

export function useComposer(
  t: ReturnType<typeof useT>,
  pressedOpen: boolean,
  pressed: PressedComposer,
  onPressedClose: () => void,
): Readonly<{
  open: boolean;
  intent: string;
  threadId: string | undefined;
  draftOnOpen: boolean;
  close: () => void;
}> {
  const [params, setParams] = useUrlParams();
  const asked = params.get(COMPOSE_PARAM);
  const key = asked ? COMPOSER_INTENT_KEYS[asked] : undefined;
  const close = () => {
    onPressedClose();
    // The address goes with it, or Back would re-open what the reader just
    // closed and the link could never be dismissed.
    if (asked) {
      const out = new Map(params);
      out.delete(COMPOSE_PARAM);
      setParams(out);
    }
  };
  return {
    open: pressedOpen || key !== undefined,
    intent: key ? t(key) : pressed.intent,
    // Which conversation, when the address named one. Read beside the intent
    // rather than folded into it: the intent is words for the reader, and this
    // decides which transport the composer opens on.
    //
    // Only for an address-opened composer. A rep who pressed Write is not
    // answering anything in particular. A thread id from a link they already
    // dismissed would anchor that press on it.
    threadId: (key !== undefined && params.get(THREAD_PARAM)) || undefined,
    // The address is the worklist's prepared move, pressed as an AI verb, so
    // that composer drafts on arrival, as a pressed AI verb's does. A press of
    // Write here starts empty.
    draftOnOpen: key !== undefined || pressed.drafts,
    close,
  };
}

// The contact page's one composer, for every contact and every entry point.
//
// The transport question is answered inside this drawer. Which way a message
// goes is a property of the message, and no reason for a different composer.
// So every caller gets the thread beside the reply, the conversation to
// continue, and a read of what the send is permitted to do.
//
// Keyed by the record: navigating to another contact while it is open remounts
// it rather than re-pointing it. Without the key, the text written for one
// contact would be filed against another.
export function ContactMailDrawer({
  contactId,
  view,
  recordAddress,
  open,
  intent,
  threadId,
  draftOnOpen,
  onClose,
}: Readonly<{
  contactId: string;
  view: Contact360;
  // Which of the contact's addresses a first message goes to. A contact carries
  // a list, so this is the shared decision rather than a field: see
  // format/primaryemail.ts, mirrored by the drafter.
  recordAddress?: string;
  open: boolean;
  /** What the action that opened it wanted written. */
  intent: string;
  /** The conversation a caller named, when one did. */
  threadId?: string;
  /** An AI verb opened it, so it drafts on open. */
  draftOnOpen?: boolean;
  onClose: () => void;
}>) {
  // Read unconditionally, above the early return: the number of hooks a render
  // performs must not depend on whether the drawer is open.
  const transports = useTransports(view);
  // Which transport that conversation is ON, rather than the contact's own
  // lead. A worklist row is about one message, and opening on whatever the
  // contact happens to lead with would draft a reply into a different thread.
  const anchored = transportForActivity(transports, view, threadId);
  // Nothing until the drawer is first opened, and mounted from then on. A
  // composer mounted with the page would read on every render, even when nobody
  // is writing. One unmounted the moment it closes has no frame to animate out on.
  const everOpened = useRef(false);
  if (open) {
    everOpened.current = true;
  }
  if (!everOpened.current) {
    return null;
  }
  return (
    <ComposeModal
      key={contactId}
      entityType="contact"
      entityId={contactId}
      contactId={contactId}
      recordAddress={recordAddress}
      transports={transports}
      initialTransportId={anchored.chosen?.id}
      staleThread={anchored.stale}
      intent={intent}
      draftOnOpen={draftOnOpen}
      open={open}
      onClose={onClose}
    />
  );
}

const ASKED_KIND = {
  activity_log: undefined,
  activity_task: "task",
  activity_call: "call",
} as const;

// The Log activity / Add task drawer: one form, one drawer state, started on
// a different kind depending on which verb opened it. companyheader.tsx's two
// LogActivityAction triggers show the same shape side by side. Split out the way ContactMailDrawer is above, so this drawer's own
// branching stays out of ContactPageV2's cognitive weight.
export function ContactActivityDrawer({
  contactId,
  drawer,
  onClose,
}: Readonly<{ contactId: string; drawer: Drawer; onClose: () => void }>) {
  if (
    drawer !== "activity_log" &&
    drawer !== "activity_task" &&
    drawer !== "activity_call"
  ) {
    return null;
  }
  return (
    <LogActivityAction
      entityType="contact"
      entityId={contactId}
      askedKind={ASKED_KIND[drawer]}
      openOnMount
      onClose={onClose}
    />
  );
}
