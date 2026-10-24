// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarPlus, CheckSquare, FileText } from "lucide-react";
import { useId, useRef } from "react";
import type { components } from "../../api/schema";
import { ExtensionEntries } from "../../components/extensionentries";
import { searchCompanyContacts } from "../../components/meetingattendees";
import { Button } from "../../design-system/atoms";
import { ListPopover } from "../../design-system/listpopover";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { ComposeModal } from "../mail/compose";
import { LogActivityAction } from "../records/logactivity";
import { EmailVerb } from "../records/recordemail";

// The account header's verbs, in the shape every record page draws them:
// writing first, then a hairline, then the pair that record what already
// happened, then the overflow menu (CompanyActionBadges, companyheader.tsx).
// Replaces CompanyPrimaryActions' own row, which drew the same three verbs as
// a labelled toolbar rather than the icon-first strip contactactions.tsx
// already carries for every other record.
//
// Named apart from companyactions.tsx, which is the account's OWN verbs (a
// new deal, a new project born on it) rather than the header's.

type Company = components["schemas"]["Company"];

// Which of LogActivityAction's two forms the drawer opens on: log what
// happened, or set what happens next. The page owns it, not two separate
// LogActivityAction mounts. ContactActivityDrawer in contactpage.drawers.tsx
// keeps the same one-drawer-two-doors shape for the same pair. It is exported
// because the daily brief's leading card opens the same drawer. So a rep who
// logs from the card and one who logs from the header meet one drawer.
export type ActivityDrawer = "log" | "task" | null;

export function CompanyHeaderActions({
  company,
  composerOpen,
  onComposerOpen,
  drawer,
  onDrawer,
  archivedReasonId,
}: Readonly<{
  company: Company;
  // The composer's open state belongs to the PAGE, not to this strip: the
  // drawer opens into the right rail's column, so the rail has to know it is
  // open in order to stand down.
  composerOpen: boolean;
  onComposerOpen: (open: boolean) => void;
  // The log/task drawer's own open state, on the same rule as the composer's:
  // the daily brief's leading card opens it too, off this strip, so the page
  // holds it rather than this component holding a copy the brief cannot reach.
  drawer: ActivityDrawer;
  onDrawer: (next: ActivityDrawer) => void;
  // The sentence the caller states once for the whole action strip. Both
  // groups in it refuse for the same reason on an archived record, so the
  // reason is the page's to say, not this component's to mint a second copy of.
  archivedReasonId?: string;
}>) {
  const t = useT();
  const ownReasonId = useId();
  const reasonId = archivedReasonId ?? ownReasonId;
  const archived = company.archived_at ? reasonId : undefined;
  // useCanWrite, not useCan: the two log verbs below issue a POST, and a read
  // seat is refused before RBAC is consulted, the same rule contactactions.tsx
  // states for the identical verb. Independent of `archived`: a live record a
  // seat may not write to is refused for this reason, not that one.
  const me = useMe();
  const canLog = useCanWrite("activity", "create");
  const logRefusedId = useId();
  const bookRefusedId = useId();
  // A guard that has not answered yet refuses nothing: claiming a refusal
  // `/me` has not decided is worse than a control that is briefly quiet.
  const logGrantKnown = me.data?.authorization !== undefined;
  const logRefused =
    archived ?? (logGrantKnown && !canLog ? logRefusedId : undefined);
  const logPending = !archived && !logGrantKnown;
  return (
    <>
      {archived && !archivedReasonId && (
        <p className="t-caption" id={ownReasonId}>
          {t("record.archivedReadOnly")}
        </p>
      )}
      {!archived && logGrantKnown && !canLog && (
        <p className="t-caption" id={logRefusedId}>
          {t("record.logActivityRefused")}
        </p>
      )}
      <CompanyWriteEmail
        company={company}
        open={composerOpen}
        onOpen={onComposerOpen}
        disabledReasonId={archived}
      />
      <span className="record-actions-sep" aria-hidden="true" />
      <Button
        disabled={logPending}
        reasonId={logRefused}
        onClick={() => onDrawer("log")}
      >
        <FileText aria-hidden="true" /> {t("log.title")}
      </Button>
      <Button
        disabled={logPending}
        reasonId={logRefused}
        onClick={() => onDrawer("task")}
      >
        <CheckSquare aria-hidden="true" /> {t("log.addTask")}
      </Button>
      <CompanyBookMeeting
        companyId={company.id}
        allowed={logGrantKnown && canLog && !archived}
        pending={logPending}
        reasonId={archived ?? (logGrantKnown ? bookRefusedId : undefined)}
      />
      {!archived && logGrantKnown && !canLog && (
        <p className="t-caption" id={bookRefusedId}>
          {t("scheduling.bookRefused")}
        </p>
      )}
      <ExtensionEntries
        slot="record_header.company"
        record={{ type: "company", id: company.id }}
      />
      <ExtensionEntries slot="record_mark.company" />
      {drawer && (
        <LogActivityAction
          entityType="company"
          entityId={company.id}
          askedKind={drawer === "task" ? "task" : undefined}
          openOnMount
          onClose={() => onDrawer(null)}
        />
      )}
    </>
  );
}

// CompanyBookMeeting books with a contact, so the account first asks which
// one, then hands over to that contact's own booking screen. The picker is
// drawn only once `/me` allows it, so a refusal that lands late also closes it.
function CompanyBookMeeting({
  companyId,
  allowed,
  pending,
  reasonId,
}: Readonly<{
  companyId: string;
  allowed: boolean;
  pending: boolean;
  reasonId?: string;
}>) {
  const t = useT();
  const face = (
    <>
      <CalendarPlus aria-hidden="true" /> {t("scheduling.bookContact")}
    </>
  );
  if (!allowed) {
    return (
      <Button variant="ghost" disabled={pending} reasonId={reasonId}>
        {face}
      </Button>
    );
  }
  return (
    <ListPopover
      label={face}
      title={t("scheduling.bookContact")}
      searchLabel={t("scheduling.bookPickContact")}
      search={(term) => searchCompanyContacts(companyId, term)}
      onPick={(contact, done) => {
        done();
        navigate({ screen: "book", id: `contact-${contact.id}` });
      }}
    />
  );
}

// CompanyWriteEmail opens the composer with no anchor. The modal owns the
// send, the consent gate and the refusal vocabulary; this owns only whether
// the surface is offered and the open/close state, so the account-started
// and reply surfaces stay one component.
function CompanyWriteEmail({
  company,
  open,
  onOpen,
  disabledReasonId,
}: Readonly<{
  company: Company;
  open: boolean;
  onOpen: (open: boolean) => void;
  disabledReasonId?: string;
}>) {
  // Whether the verb has ever been pressed. `open` belongs to the page, so
  // this is the composer's own memory of having been asked for.
  const everOpened = useRef(false);
  if (open) {
    everOpened.current = true;
  }
  return (
    <>
      {/* The shared Email verb: icon-only with its name on hover, the same
          control every record header draws, found by its place and its word. */}
      <EmailVerb reasonId={disabledReasonId} onClick={() => onOpen(true)} />
      {/* Not drawn until the verb has been pressed once, and mounted from
          then on. A composer mounted with the record would read on every
          render of a page nobody is writing from; one unmounted the moment it
          closes has no frame left to animate out on. */}
      {everOpened.current && (
        // Keyed by the record, so navigating to another company while the
        // composer is open remounts it rather than re-pointing it. Without
        // the key the form keeps the text written for the previous account
        // while the links payload follows the new one.
        <ComposeModal
          key={company.id}
          entityType="company"
          entityId={company.id}
          open={open}
          onClose={() => onOpen(false)}
        />
      )}
    </>
  );
}
