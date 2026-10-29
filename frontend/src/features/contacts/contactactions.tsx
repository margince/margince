// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, FileText } from "lucide-react";
import type { ReactNode } from "react";
import {
  type ActivityGate,
  useActivityGate,
} from "../../components/activitygate";
import { AddToShortlistAction } from "../../components/addtoshortlist";
import { ExtensionEntries } from "../../components/extensionentries";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { IconAction, VerbWord } from "../../design-system/iconaction";
import { useT } from "../../i18n";
import type { Transport, TransportSource } from "../../lib/contacttransports";
import {
  primaryTransportAction,
  useTransports,
} from "../../lib/contacttransports";
import { navigate } from "../../lib/router";
import { EmailVerb } from "../records/recordemail";
import { ShareAction } from "../records/share";
import { openContactTab } from "../worklist/worklist.return";
import { ImportActivitiesAction } from "./contactimportaction";
import { ContactRecordActions } from "./contactrecordactions";

// The header's verbs on the contact record page (contactpage.tsx): meeting,
// logging, and the menu. Writing and calling sit beside the address and the
// number in the contact's details.

// Why writing to the contact is refused, or undefined when it is not. Only
// reachability refuses it: with nowhere to write to, a send could only fail.
// Consent is the composer's to ask: it is decided per purpose, and the purpose
// belongs to the message. contactpage.test.tsx holds this ("opens the composer
// even where no purpose currently permits writing").
function writeRefusal(
  state: Readonly<{ transports: readonly Transport[] }>,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (state.transports.length === 0) {
    return t("contact.action.noTransport");
  }
  return undefined;
}

/** Why the write icon may not be pressed, for the page's footnote. */
export function useWriteRefusal(view: TransportSource): string | undefined {
  const t = useT();
  return writeRefusal({ transports: useTransports(view) }, t);
}

// The header's verbs. None of them is filled: the move worth doing is the one
// the call names, and that one carries the colour.
export function ContactActions({
  view,
  contactId,
  onWrite,
  onResearch,
  onLogActivity,
  onAddTask,
  activity,
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
  activity?: ActivityGate;
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
  return (
    <>
      <ChatVerb view={view} onWrite={onWrite} refusedReasonId={archived} />
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
        {/* First in the menu: the most frequent verb here. Files the task
            against THIS record, in the same form Log activity opens. */}
        <Button disabled={logPending} reasonId={logRefused} onClick={onAddTask}>
          {t("contact.action.addTask")}
        </Button>
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

// The write verb for a contact reached only on chat channels, named for the
// channel it opens. An address carries its own icon in the details. A contact
// with no channel at all gets the footnote's sentence instead.
function ChatVerb({
  view,
  onWrite,
  refusedReasonId,
}: Readonly<{
  view: TransportSource;
  onWrite: () => void;
  refusedReasonId: string | undefined;
}>) {
  const t = useT();
  const transports = useTransports(view);
  if (view.contact.primary_email || transports.length === 0) {
    return null;
  }
  const write = primaryTransportAction(transports, t);
  const WriteIcon = write.icon;
  return (
    <EmailVerb
      label={write.label}
      icon={<WriteIcon size={15} aria-hidden="true" />}
      reasonId={refusedReasonId}
      onClick={onWrite}
      word={t("record.verb.email")}
    />
  );
}
