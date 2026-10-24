import {
  CalendarDays,
  Check,
  CheckSquare,
  FileText,
  Phone,
} from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";
import { AddToShortlistAction } from "../../components/addtoshortlist";
import { EntityRef } from "../../components/entityref";
import { LogActivityAction } from "../../components/logactivity.action";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { IconAction, VerbWord } from "../../design-system/iconaction";
import { leadIdentityName } from "../../format/leadname";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useMe } from "../../lib/session";
import { useUrlParams } from "../../lib/urlstate";
import { RecordEmailVerb } from "../records/recordemail";
import { ShareAction } from "../records/share";
import { ReopenAction, terminalSentence } from "./leadband";
import { LeadFillFromContact } from "./leadfillfromcontact";
import { promoteEligible } from "./leadpresentation";
import type { Lead, LeadWriter } from "./leads";
import { ACTION_PARAM, CALL_ACTION } from "./leads.address";
import { DisqualifyDialog } from "./leads.disqualify";
import { DemoteAction } from "./leads.promotion";
import { QualifyDialog } from "./leads.qualify";

// A closed lead says so in the verdict of How it stands. This is the sentence
// its refused controls point at, kept for assistive technology rather than
// drawn a second time.
function ClosedReason({
  lead,
  writer,
  id,
}: Readonly<{ lead: Lead; writer: LeadWriter; id: string }>) {
  const t = useT();
  const sentence = lead.archived_at
    ? terminalSentence(lead, writer, t)
    : undefined;
  return sentence ? (
    <p id={id} className="sr-only">
      {sentence}
    </p>
  ) : null;
}

// The lead's identity row and its verbs. It stands apart from LeadScreen to
// keep that render inside the complexity budget. A header is a unit of its own:
// the name, why the verbs are gone when they are, and the verbs themselves.
export function LeadActions({
  lead,
  id,
  writer,
  onQualify,
  onDisqualify,
  onLogged,
  terminalReasonId,
  refusedReasonId,
  emailOpen,
  onEmailOpenChange,
}: Readonly<{
  lead: Lead;
  id: string;
  // Fill from a contact saves through the page's one lead write.
  writer: LeadWriter;
  onQualify: () => void;
  onDisqualify: () => void;
  // Fires once the Log activity/Add task drawer logs something. The page then
  // re-polls the ladder fields an activity can move (status, score, the next
  // task), the same rule the drawer's own `onLogged` states.
  onLogged: () => void;
  // The id of the one sentence this page prints about why the lead takes no
  // changes. Every refused control points at it rather than repeating it,
  // which is what stops a terminal lead printing the same line five times.
  terminalReasonId: string;
  // That same id while the sentence is on the page, and undefined while the
  // lead takes writes. The sentence shows when the lead is closed or not this
  // caller's to change. Email writes no lead row, so it reads closure alone.
  refusedReasonId?: string;
  // The header's composer, lifted to LeadRecord: the "Answer" row's own Reply
  // verb opens this same state rather than a copy of it.
  emailOpen: boolean;
  onEmailOpenChange: (open: boolean) => void;
}>) {
  const t = useT();
  // useCanWrite, not useCan: the drawer below issues a POST, and a read seat is
  // refused before RBAC is consulted. The contact and company headers state the
  // same rule for this pair of verbs, in contactactions.tsx and its sibling.
  // This is independent of the terminal check. A live lead a seat may not log
  // against is refused for this reason, not that one.
  const me = useMe();
  const canLog = useCanWrite("activity", "create");
  const logRefusedId = useId();
  // A guard that has not answered yet refuses nothing: claiming a refusal
  // `/me` has not decided is worse than a control that is briefly quiet.
  const logGrantKnown = me.data?.authorization !== undefined;
  const archived = lead.archived_at ? terminalReasonId : undefined;
  const logRefused =
    archived ?? (logGrantKnown && !canLog ? logRefusedId : undefined);
  const logPending = !archived && !logGrantKnown;
  // The verb the reader arrived to perform, named by the address rather than
  // guessed. A link sent here to log a call says so. The drawer then opens on
  // the call instead of the note they would have to change.
  //
  // It stays in the address, like every other dial this product carries. The
  // link is one somebody can paste, and Back returns to the screen it named.
  const [params] = useUrlParams();
  const askedToLogCall = params.get(ACTION_PARAM) === CALL_ACTION;
  const [drawer, setDrawer] = useState<"log" | "call" | "meet" | "task" | null>(
    askedToLogCall ? "call" : null,
  );
  // A link pressed on the record the reader is already on changes only the
  // address, with no remount. So the initializer above never runs again.
  useEffect(() => {
    if (askedToLogCall) {
      setDrawer("call");
    }
  }, [askedToLogCall]);
  return (
    <>
      <ClosedReason lead={lead} writer={writer} id={terminalReasonId} />
      {/* Promote is the page's one primary action and it leads, in the header
          where a reader looks for the verb (ADR-0108). Ineligibility is stated
          on the control itself: a disabled button whose reason is elsewhere is
          a dead button. */}
      {!lead.archived_at && (
        <Button
          variant="primary"
          data-testid="lead-qualify"
          reasonId={refusedReasonId}
          reason={
            promoteEligible(lead) ? undefined : t("lead.promoteIneligible")
          }
          onClick={onQualify}
        >
          {/* No `size`: the button owns its icon's geometry, and a call site
              that names one is a second author of it. */}
          <Check aria-hidden="true" /> {t("lead.promote")}
        </Button>
      )}
      {/* The shared Email verb every record header carries. Its open state is
          lifted so the overview's "Answer" row can open this same composer. */}
      <RecordEmailVerb
        entityType="lead"
        entityId={id}
        recordAddress={lead.email ?? undefined}
        disabledReasonId={archived}
        open={emailOpen}
        onOpenChange={onEmailOpenChange}
      />
      {/* A lead has no phone screen or calendar of its own, so Call and
          Meet record what happened, the drawer opened on that kind. */}
      <IconAction
        label={t("contact.action.call")}
        word={t("record.verb.call")}
        icon={<Phone aria-hidden="true" />}
        disabled={logPending}
        reasonId={logRefused}
        onClick={() => setDrawer("call")}
      />
      <IconAction
        label={t("contact.action.meetings")}
        word={t("record.verb.meet")}
        icon={<CalendarDays aria-hidden="true" />}
        disabled={logPending}
        reasonId={logRefused}
        onClick={() => setDrawer("meet")}
      />
      {!archived && logGrantKnown && !canLog && (
        <p id={logRefusedId}>{t("record.logActivityRefused")}</p>
      )}
      {/* A CRM a rep cannot write a meeting into is a CRM that only reads.
          This is the standing way in, the same pair contactactions.tsx and
          companyheaderactions.tsx carry: what happened, and what happens
          next. The drawer stays local to this header, unlike company's own
          daily-brief card, because nothing else on the lead page opens it. */}
      <Button
        disabled={logPending}
        reasonId={logRefused}
        title={t("log.title")}
        onClick={() => setDrawer("log")}
      >
        <FileText aria-hidden="true" />
        <VerbWord word={t("record.verb.log")} label={t("log.title")} />
      </Button>
      <Button
        disabled={logPending}
        reasonId={logRefused}
        title={t("log.addTask")}
        onClick={() => setDrawer("task")}
      >
        <CheckSquare aria-hidden="true" />
        <VerbWord word={t("record.verb.task")} label={t("log.addTask")} />
      </Button>
      {drawer && (
        <LogActivityAction
          entityType="lead"
          entityId={id}
          askedKind={
            drawer === "task" || drawer === "call" ? drawer : undefined
          }
          openOnMount
          onLogged={onLogged}
          onClose={() => setDrawer(null)}
        />
      )}
      {/* Everything else this lead offers, behind one trigger. Qualify and
          Email are what a rep reaches for between calls; the rest are rare
          enough that a reader hunting one of them should not have to read
          past them to find a common verb. Worded and glyphless in here: a
          list of named actions with one unnamed square in it makes that
          square the only row a reader has to hover to identify.

          A terminal lead keeps these controls, disabled with the reason:
          hiding the control hides a fact the reader needs. Both closures
          (disqualified and promoted) reach this page, and the band above
          names which. The sentence is passed in because a reason minted
          in the panel would not exist until the menu was first opened. */}
      <OverflowMenu
        label={t("record.moreActions")}
        word={t("record.verb.more")}
      >
        {/* The ways back out of a closure, offered by the closure that was
            taken: a promotion is the demote's to reverse, a disqualification
            the reopen's. Neither is on a live lead. */}
        {lead.promoted_contact_id && <DemoteAction id={lead.id} />}
        <ReopenAction lead={lead} />
        <LeadFillFromContact writer={writer} reasonId={refusedReasonId} />
        <ShareAction
          recordType="lead"
          recordId={lead.id}
          disabledReasonId={refusedReasonId}
        />
        <AddToShortlistAction entityType="lead" entityId={lead.id} />
        {/* Last: it is the one verb here a reader cannot walk back from
                the header, so it does not sit where a pointer sliding down
                the list reaches it on the way to something routine. It asks
                why, in its own dialog, and stays a secondary verb rather than
                a red one. Closing a lead is ordinary work, and the panel's
                seam (atoms.css) belongs to the destructive verbs. A terminal
                lead keeps the control, disabled with the page's one reason. */}
        <Button
          data-testid="lead-disqualify"
          reasonId={refusedReasonId}
          onClick={onDisqualify}
        >
          {t("record.disqualify")}
        </Button>
      </OverflowMenu>
    </>
  );
}

// The two governed-transition dialogs, mounted only while open. They are keyed
// by the lead, so a half-filled deal block never carries to the next lead. The
// qualify outcome comes back as the sentence the page shows.
export function LeadDialogs({
  lead,
  dialog,
  onClose,
  onQualified,
}: Readonly<{
  lead: Lead;
  dialog: "qualify" | "disqualify" | null;
  onClose: () => void;
  onQualified: (done: ReactNode) => void;
}>) {
  const t = useT();
  if (dialog === "qualify") {
    return (
      <QualifyDialog
        key={`qualify-${lead.id}`}
        lead={lead}
        open
        onClose={onClose}
        onQualified={(result) =>
          onQualified(
            <span>
              {t("lead.qualify.done", {
                name: leadIdentityName(lead),
              })}{" "}
              <EntityRef kind="contact" id={result.contact.id} />
              {result.deal_id && (
                <>
                  {" · "}
                  <EntityRef kind="deal" id={result.deal_id} />
                </>
              )}
            </span>,
          )
        }
      />
    );
  }
  if (dialog === "disqualify") {
    return (
      <DisqualifyDialog
        key={`disqualify-${lead.id}`}
        lead={lead}
        open
        onClose={onClose}
        onDisqualified={onClose}
      />
    );
  }
  return null;
}
