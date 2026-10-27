// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, CheckSquare, FileText } from "lucide-react";
import type { ReactNode } from "react";
import { useId } from "react";
import { AddToShortlistAction } from "../../components/addtoshortlist";
import { ExtensionEntries } from "../../components/extensionentries";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { CallAction } from "../../design-system/callaction";
import { IconAction, VerbWord } from "../../design-system/iconaction";
import { primaryPhone } from "../../format/contacturi";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import type { Transport, TransportSource } from "../../lib/contacttransports";
import {
  primaryTransportAction,
  useTransports,
} from "../../lib/contacttransports";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { EmailVerb } from "../records/recordemail";
import { ShareAction } from "../records/share";
import { openContactTab } from "../worklist/worklist.return";
import { ImportActivitiesAction } from "./contactimportaction";
import { ContactRecordActions } from "./contactrecordactions";

// The header's verbs on the contact record page (contactpage.tsx): writing,
// calling, meeting, logging, and the menu that holds everything else.

// Why the lead verb may not be pressed, in the reader's words, or undefined
// when it may.
//
// ONE fact refuses it: reachability. There is nowhere to write to, so the
// composer would have nothing to send on and could only fail at the send.
//
// CONSENT IS NOT ASKED HERE, and that is the decision rather than a gap. A
// consent verdict is reached per PURPOSE, and which purpose applies is a fact
// about the MESSAGE — which this page does not have. Disabling the verb on one
// purpose's verdict told a rep with a lawful service message to send that the
// product would not let them write at all, with no way to see that only
// marketing was refused. The composer asks, because the composer is where the
// purpose is chosen.
//
// So the two facts never merge into one sentence because only one of them is a
// sentence here. Held by "opens the composer even where no purpose currently
// permits writing" (contactpage.test.tsx), which asserts the verb carries no
// refusal at all for a consent-blocked contact who is reachable.
function writeRefusal(
  state: Readonly<{ transports: readonly Transport[] }>,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (state.transports.length === 0) {
    return t("contact.action.noTransport");
  }
  return undefined;
}

/** Why the Email verb may not be pressed, for the page's footnote. */
export function useWriteRefusal(view: TransportSource): string | undefined {
  const t = useT();
  return writeRefusal({ transports: useTransports(view) }, t);
}

/**
 * Whether this reader may write activities (log, task, book), asked once for
 * the page. The verbs point at `sentenceId`, and the page states the refusal
 * under that id in the left card's footnote. A grant `/me` has not answered
 * yet refuses nothing; the verbs are only briefly quiet (`pending`).
 */
export function useActivityGate(): Readonly<{
  refusedId: string | undefined;
  pending: boolean;
  sentenceId: string;
}> {
  // useCanWrite, not useCan: Log and Add task issue the same POST, and a read
  // seat is refused before RBAC is consulted.
  const me = useMe();
  const canLog = useCanWrite("activity", "create");
  const sentenceId = useId();
  const known = me.data?.authorization !== undefined;
  return {
    refusedId: known && !canLog ? sentenceId : undefined,
    pending: !known,
    sentenceId,
  };
}

// The header's verbs, in the order every record page carries them: writing
// first, then the record's other doors. None of them is filled — the move
// worth doing is the one the call names, and that one carries the colour.
export function ContactActions({
  view,
  contactId,
  onWrite,
  onResearch,
  onLogActivity,
  onAddTask,
  activity,
  writeRefusalId,
  refusedReasonId,
}: Readonly<{
  view: TransportSource;
  contactId: string;
  onWrite: () => void;
  onResearch: () => void;
  onLogActivity: () => void;
  onAddTask: () => void;
  // The page's gate, whose refusal the page states. A host that passes none
  // (the archived page, whose own sentence refuses every verb) gets its own.
  activity?: ReturnType<typeof useActivityGate>;
  // Where the page states why Email is refused, when it does. Absent, the
  // verb states it under itself.
  writeRefusalId?: string;
  // The page's one sentence about why this contact takes no changes, while
  // it does not. The verbs that write the RECORD — edit, merge, archive and
  // the grant Share asserts — are the ones refused by it; logging and mail
  // are activity writes with gates of their own.
  refusedReasonId?: string;
}>): ReactNode {
  const t = useT();
  const ownGate = useActivityGate();
  const gate = activity ?? ownGate;
  // An archived contact takes no activity either, and the page's one sentence
  // says so for every verb here.
  const archived = view.contact.archived_at ? refusedReasonId : undefined;
  const logRefused = archived ?? gate.refusedId;
  const logPending = !archived && gate.pending;
  // The transports the composer would offer, read here so the button NAMES
  // what pressing it does. The same reachability the drawer resolves: a label
  // computed from anything else is a promise the composer then breaks.
  const transports = useTransports(view);
  const write = primaryTransportAction(transports, t);
  const WriteIcon = write.icon;
  const refusal = writeRefusal({ transports }, t);
  return (
    <>
      {/* The shared Email verb, wearing the transport it will open when there
          is exactly one, neutral when the composer will ask, and explaining
          itself rather than merely dimming when it may not be pressed. */}
      <EmailVerb
        label={write.label}
        icon={<WriteIcon size={15} aria-hidden="true" />}
        reason={archived || writeRefusalId ? undefined : refusal}
        reasonId={archived ?? (refusal ? writeRefusalId : undefined)}
        onClick={onWrite}
        word={t("record.verb.email")}
      />
      {/* Square, because a phone and a calendar are verbs a reader already
          knows from the glyph — and five labelled buttons in a row is a header
          that reads as a toolbar, with the one action the page is FOR no more
          prominent than the rest of them. `IconAction` owes each one its name
          on hover as well as to a screen reader. */}
      <CallAction
        appearance="icon"
        number={primaryPhone(view.contact.phones)?.phone}
        record={{ entityType: "contact", entityId: contactId }}
        label={t("contact.action.call")}
        word={t("record.verb.call")}
        reason={t("contact.action.noPhone")}
        reasonId={archived}
      />
      <IconAction
        label={t("contact.action.meetings")}
        word={t("record.verb.meet")}
        icon={<CalendarDays aria-hidden="true" />}
        reasonId={archived}
        onClick={() => openContactTab(contactId, "meetings")}
      />
      {/* A CRM a rep cannot write a meeting into is a CRM that only reads.
          This is the standing way in; the moment card offers the same form
          when its rung decides logging is the thing to do next. */}
      <Button
        disabled={logPending}
        reasonId={logRefused}
        onClick={onLogActivity}
        title={t("log.title")}
      >
        <FileText aria-hidden="true" />
        <VerbWord word={t("record.verb.log")} label={t("log.title")} />
      </Button>
      {/* Keeps its words. A tick box is the glyph for COMPLETING a task, so
          squaring this one would name the opposite of what it does. Files the
          task against THIS record — the same form Log activity opens, started
          on its task kind, rather than a navigation to the Worklist, which has
          no way to add one. */}
      <Button
        disabled={logPending}
        reasonId={logRefused}
        onClick={onAddTask}
        title={t("contact.action.addTask")}
      >
        <CheckSquare aria-hidden="true" />
        <VerbWord
          word={t("record.verb.task")}
          label={t("contact.action.addTask")}
        />
      </Button>
      <ExtensionEntries
        slot="record_header.contact"
        record={{ type: "contact", id: contactId }}
      />
      <ExtensionEntries slot="record_mark.contact" />
      {/* Every secondary verb, behind one control. A header that put edit,
          merge and archive beside the daily verbs made the destructive one as
          easy to reach as the routine one, and read as a toolbar rather than
          as a record with something to do. Each row keeps its WORDS and no
          glyph: a list of named actions with one picture in it is a list with
          one row a reader has to decode.

          Research is here because a magnifier reads as "search" and this verb
          is not search, and the timeline gets the honest name the product
          already uses for it everywhere else. */}
      <OverflowMenu
        label={t("record.moreActions")}
        word={t("record.verb.more")}
      >
        <ContactRecordActions
          contact={view.contact}
          disabledReasonId={refusedReasonId}
          beforeArchive={
            <>
              {/* Companies, deals, leads and projects all carry this. A contact
                  did not, so the one record type most likely to be private to
                  one seat was the one with no way to hand it to a colleague. */}
              <Button
                onClick={() =>
                  navigate({ screen: "book", id: `contact-${contactId}` })
                }
                disabled={logPending}
                reasonId={logRefused}
              >
                {t("scheduling.new")}
              </Button>
              <ShareAction
                recordType="contact"
                recordId={contactId}
                disabledReasonId={refusedReasonId}
              />
              <AddToShortlistAction
                entityType="contact"
                entityId={contactId}
                reasonId={archived}
              />
              <Button onClick={() => openContactTab(contactId, "timeline")}>
                {t("record.fullHistory")}
              </Button>
              <ImportActivitiesAction
                contactId={contactId}
                reasonId={archived}
              />
              <Button reasonId={archived} onClick={onResearch}>
                {t("contact.action.research")}
              </Button>
            </>
          }
        />
      </OverflowMenu>
    </>
  );
}
