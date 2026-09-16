import { Mail, Phone } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { Button, Modal, Skeleton } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { useProviderLabel } from "./channelproviders";
import { ConfirmDetailsAction, ConsentSection } from "./consent";
import { consentWord } from "./contactreadings";
import { interactionIcon } from "./interactionchrome";

type Contact360 = components["schemas"]["Contact360"];
type ContactConsentGuard = components["schemas"]["ContactConsentGuard"];

// The guard describes communication purposes; the drawer holds recorded consent.
export function ConsentAndChannels({
  view,
  guard,
  loading = false,
  failed = false,
  onRetry,
}: Readonly<{
  view: Contact360;
  guard: ContactConsentGuard | undefined;
  loading?: boolean;
  failed?: boolean;
  onRetry?: () => void;
}>) {
  const t = useT();
  const providerLabel = useProviderLabel();
  const [manage, setManage] = useState(false);
  const titleId = useId();
  const mayWrite = useCanWriteRecord("contact", view.contact);
  const entries = guard?.entries ?? [];
  // WHICH email purpose. The guard answers one verdict per purpose, and taking
  // the first of them painted the Email row with whichever the server happened
  // to list first — a bare "Allowed" that the composer then contradicted with
  // "sending will be refused until Margince has a record", because the two were
  // answering about different purposes and neither said which.
  //
  // Correspondence is the one a rail can speak for: it is the purpose a reply
  // rides, and the only one an inbound message can flip on its own. The others
  // get their own rows below, each carrying its name.
  const correspondence =
    entries.find(
      (entry) => entry.purpose_class === "business_correspondence",
    ) ?? entries.find((entry) => entry.channel === "email");
  const otherPurposes = entries.filter(
    (entry) => entry.channel === "email" && entry !== correspondence,
  );
  const phone = entries.find((entry) => entry.channel === "phone");
  const emails = view.contact.emails ?? [];
  const hasEmail = emails.length > 0;
  const knownRecipient =
    emails.length === 1 || emails.some((email) => email.is_primary);
  const channels = view.contact.reachability ?? [];
  return (
    <Panel title={t("contact.rail.consentTitle")}>
      <PanelBody>
        {loading ? (
          <Skeleton width="100%" />
        ) : failed ? (
          <p role="alert">
            {t("consent.guardFailed")}{" "}
            <Button variant="ghost" onClick={onRetry}>
              {t("common.retry")}
            </Button>
          </p>
        ) : (
          <>
            <ConsentRow
              icon={<Mail size={15} aria-hidden="true" />}
              label={correspondence?.purpose_label ?? t("contact.rail.email")}
              reachable={hasEmail}
              verdict={correspondence?.verdict}
              reason={correspondence?.reason}
              unreachableWord={t("contact.rail.noEmailAddress")}
            />
            <ConsentRow
              icon={<Phone size={15} aria-hidden="true" />}
              label={t("contact.rail.phone")}
              reachable={(view.contact.phones?.length ?? 0) > 0}
              verdict={phone?.verdict}
              unreachableWord={t("contact.rail.noPhoneNumber")}
            />
            {/* A blocked identity still gets its row, with `reachable: false`: the
          conversation happened, and hiding the transport it happened on would
          answer "can I write to them" by pretending they were never here. */}
            {channels.map((channel) => (
              <ConsentRow
                key={channel.provider}
                icon={interactionIcon("message", 15)}
                label={providerLabel(channel.provider)}
                reachable={channel.reachable}
                verdict={correspondence?.verdict}
                unreachableWord={t("contact.rail.channelNotDeliverable")}
              />
            ))}
            {/* Every OTHER purpose, by name. A rep who reads "Allowed" against
          Email and is then refused at the composer has been told two true
          things and no way to reconcile them: the grant they have is for
          correspondence and the send they tried was something else. Naming
          each purpose is what makes the two answers agree on screen. */}
            {/* EACH PURPOSE CARRIES ITS OWN REASON, under the row it explains. The
          rail used to print one reason — correspondence's — for the whole
          panel, so a subject who asked us to stop marketing got a blocked row
          with nothing saying why. A refusal a rep cannot explain to the contact
          in front of them is not usable. */}
            {hasEmail &&
              otherPurposes.map((entry) => (
                <ConsentRow
                  key={entry.purpose_key}
                  icon={<Mail size={15} aria-hidden="true" />}
                  label={entry.purpose_label ?? entry.purpose_key}
                  reachable
                  verdict={entry.verdict}
                  reason={entry.reason}
                  unreachableWord={t("contact.rail.noEmailAddress")}
                />
              ))}
          </>
        )}
        <p>{t("consent.permissionScope")}</p>
        <Button variant="ghost" onClick={() => setManage(true)}>
          {t("consent.manage")}
        </Button>
      </PanelBody>
      <ConfirmDetailsAction
        key={view.contact.id}
        contactId={view.contact.id}
        mayWrite={mayWrite}
        recipient={knownRecipient ? view.contact.primary_email : undefined}
        unavailableReason={
          !hasEmail ? t("contact.rail.noEmailAddress") : undefined
        }
      />
      {manage && (
        <Modal
          open
          onClose={() => setManage(false)}
          labelledBy={titleId}
          placement="right"
        >
          <div className="pe-drawer-title">
            <Heading size="large" id={titleId}>
              {t("consent.manage")}
            </Heading>
            <Button variant="ghost" onClick={() => setManage(false)}>
              {t("common.close")}
            </Button>
          </div>
          <ConsentSection
            contactId={view.contact.id}
            contact={view.contact}
            showConfirm={false}
            titleLevel={3}
          />
        </Modal>
      )}
    </Panel>
  );
}

// One transport's row. Reachability is a fact about the RECORD and the verdict
// is a fact about consent, and the row states the first before the second: a
// permission to send where there is nowhere to send is not one a rep can act
// on, and colouring it green says they may.
function ConsentRow({
  icon,
  label,
  reachable,
  verdict,
  reason,
  unreachableWord,
}: Readonly<{
  icon: ReactNode;
  label: string;
  reachable: boolean;
  verdict: string | undefined;
  // Why this purpose answers as it does, under the row it explains. Absent for
  // the transports, which answer on reachability and need no sentence.
  reason?: string;
  unreachableWord: string;
}>) {
  const t = useT();
  return (
    <>
      <div className="pe-rail-row">
        <span className="pe-rail-label">
          {icon}
          {label}
        </span>
        <span
          className={
            reachable
              ? verdictClass(verdict)
              : "pe-rail-value pe-rail-value-muted"
          }
        >
          {reachable ? consentWord(verdict, t) : unreachableWord}
        </span>
      </div>
      {/* Only where the row shows a verdict: a sentence explaining an answer
        the row above replaced with "no address" explains nothing. */}
      {reachable && reason && (
        <p className="pe-colleague-proof t-caption">{reason}</p>
      )}
    </>
  );
}

function verdictClass(verdict: string | undefined): string {
  switch (verdict) {
    case "allowed":
      return "pe-rail-value pe-rail-value-good";
    case "blocked":
      return "pe-rail-value pe-rail-value-warn";
    default:
      return "pe-rail-value pe-rail-value-muted";
  }
}
