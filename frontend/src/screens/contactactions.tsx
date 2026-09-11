// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, CheckSquare, FileText, Phone } from "lucide-react";
import type { ReactNode } from "react";
import { useId } from "react";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { navigate } from "../app/router";
import { Button, OverflowMenu } from "../design-system/atoms";
import { IconAction } from "../design-system/iconaction";
import { useT } from "../i18n";
import { useMe } from "./common";
import { ContactEditMergeArchive } from "./contacteditmergearchive";
import { contactTabRoute } from "./contacttab";
import type { Transport } from "./contacttransports";
import { primaryTransportAction, useTransports } from "./contacttransports";
import type { ObjectCustomFields } from "./customfields.form";
import { EmailVerb } from "./recordemail";
import { ShareAction } from "./share";

// The header's verbs on the contact record page (contactpage.tsx): writing,
// calling, meeting, logging, and the menu that holds everything else.

type Contact360 = components["schemas"]["Contact360"];

// Why the lead verb may not be pressed, in the reader's words, or undefined
// when it may.
//
// TWO facts refuse it and they are never merged into one sentence: consent says
// we may not write to this contact, reachability says there is nowhere to write
// to. A rep who is told the wrong one goes looking in the wrong record.
//
// Reachability is asked first because it is the unconditional half — with no
// transport the composer has nothing to send on whatever consent says, and a
// consent sentence there would describe a decision that is not what stops them.
// A guard that has not answered yet refuses nothing: the button is disabled
// without a reason until the verdict is in, because claiming a refusal the
// server has not made is worse than a control that is briefly quiet.
function writeRefusal(
  state: Readonly<{ transports: readonly Transport[] }>,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (state.transports.length === 0) {
    return t("contact.action.noTransport");
  }
  return undefined;
}

// The header's verbs, in the order every record page carries them: writing
// first, then the record's other doors. None of them is filled — the move
// worth doing is the one the call names, and that one carries the colour.
export function ContactActions({
  view,
  contactId,
  cf,
  overlay,
  onWrite,
  onResearch,
  onLogActivity,
  onAddTask,
  refusedReasonId,
}: Readonly<{
  view: Contact360;
  contactId: string;
  // Read at screen level and handed down so the custom-field schema request
  // runs BESIDE the contact's. See ContactEditMergeArchive's own prop.
  cf: ObjectCustomFields;
  // LogActivityAction itself renders nothing in overlay — a mirrored
  // workspace has no activity write of its own, the same fact
  // ContactEmailPanel already states for the record's email box — so a
  // trigger drawn here would set drawer state a mount elsewhere refuses.
  overlay: boolean;
  onWrite: () => void;
  onResearch: () => void;
  onLogActivity: () => void;
  onAddTask: () => void;
  // The page's one sentence about why this contact takes no changes, while
  // it does not. The verbs that write the RECORD — edit, merge, archive and
  // the grant Share asserts — are the ones refused by it; logging and mail
  // are activity writes with gates of their own.
  refusedReasonId?: string;
}>): ReactNode {
  const t = useT();
  // useCanWrite, not useCan: both this verb and Add task below issue the same
  // POST, and a read seat is refused before RBAC is consulted. The buttons
  // stay on the page and say why they will not press, so a reader can tell
  // "not mine to do" from "this build has no such button".
  const me = useMe();
  const canLog = useCanWrite("activity", "create");
  // Named once and pointed at by both buttons — Button's own contract for a
  // surface where several controls are refused by ONE fact: printing the
  // sentence beside each button says it as many times as there are buttons.
  const logRefusedId = useId();
  // A guard that has not answered yet refuses nothing: claiming a refusal
  // `/me` has not decided is worse than a control that is briefly quiet —
  // the same rule writeRefusal states for the identical shape, above.
  const logGrantKnown = me.data?.authorization !== undefined;
  const logRefused = logGrantKnown && !canLog ? logRefusedId : undefined;
  const logPending = !logGrantKnown;
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
        reason={refusal}
        onClick={onWrite}
      />
      {/* Square, because a phone and a calendar are verbs a reader already
          knows from the glyph — and five labelled buttons in a row is a header
          that reads as a toolbar, with the one action the page is FOR no more
          prominent than the rest of them. `IconAction` owes each one its name
          on hover as well as to a screen reader. */}
      <IconAction
        label={t("contact.action.call")}
        icon={<Phone size={15} aria-hidden="true" />}
        onClick={() => navigate(contactTabRoute(contactId, "timeline"))}
      />
      <IconAction
        label={t("contact.action.meetings")}
        icon={<CalendarDays size={15} aria-hidden="true" />}
        onClick={() =>
          navigate({ screen: "contacts", id: contactId, id2: "meetings" })
        }
      />
      {/* Neither verb is drawn in overlay: LogActivityAction, the form both
          open, renders nothing there — a mirrored workspace has no activity
          write of its own — so a trigger here would set drawer state a mount
          elsewhere refuses to act on. */}
      {!overlay && (
        <>
          {logRefused && (
            <p className="t-caption" id={logRefusedId}>
              {t("record.logActivityRefused")}
            </p>
          )}
          {/* A CRM a rep cannot write a meeting into is a CRM that only
              reads. This is the standing way in; the moment card offers the
              same form when its rung decides logging is the thing to do
              next. */}
          <Button
            disabled={logPending}
            reasonId={logRefused}
            onClick={onLogActivity}
          >
            <FileText size={15} aria-hidden="true" /> {t("log.title")}
          </Button>
          {/* Keeps its words. A tick box is the glyph for COMPLETING a task,
              so squaring this one would name the opposite of what it does.
              Files the task against THIS record — the same form Log activity
              opens, started on its task kind, rather than a navigation to the
              Worklist, which has no way to add one. */}
          <Button
            disabled={logPending}
            reasonId={logRefused}
            onClick={onAddTask}
          >
            <CheckSquare size={15} aria-hidden="true" />{" "}
            {t("contact.action.addTask")}
          </Button>
        </>
      )}
      {/* Every secondary verb, behind one control. A header that put edit,
          merge and archive beside the daily verbs made the destructive one as
          easy to reach as the routine one, and read as a toolbar rather than
          as a record with something to do. Each row keeps its WORDS and no
          glyph: a list of named actions with one picture in it is a list with
          one row a reader has to decode.

          Research is here because a magnifier reads as "search" and this verb
          is not search, and the timeline gets the honest name the product
          already uses for it everywhere else. */}
      <OverflowMenu label={t("record.moreActions")}>
        <ContactEditMergeArchive
          contact={view.contact}
          cf={cf}
          disabledReasonId={refusedReasonId}
          overlay={overlay}
          beforeArchive={
            <>
              {/* Companies, deals, leads and projects all carry this. A contact
                  did not, so the one record type most likely to be private to
                  one seat was the one with no way to hand it to a colleague. */}
              <ShareAction
                recordType="contact"
                recordId={contactId}
                disabledReasonId={refusedReasonId}
              />
              <Button
                small
                onClick={() => navigate(contactTabRoute(contactId, "timeline"))}
              >
                {t("record.fullHistory")}
              </Button>
              <Button small onClick={onResearch}>
                {t("contact.action.research")}
              </Button>
            </>
          }
        />
      </OverflowMenu>
    </>
  );
}
