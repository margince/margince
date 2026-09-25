import { Mail, Phone } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { Button, Modal, Skeleton } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { type Translator, useT } from "../i18n";
import { useProviderLabel } from "./channelproviders";
import { ConfirmDetailsAction, ConsentSection } from "./consent";
import { consentWord } from "./contactreadings";
import { interactionIcon } from "./interactionchrome";

type Contact360 = components["schemas"]["Contact360"];
type ContactConsentGuard = components["schemas"]["ContactConsentGuard"];
type GuardEntry = components["schemas"]["ContactConsentGuardEntry"];
type Channel = GuardEntry["channel"];

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
  // Opened once, mounted from then on. The drawer's section reads consent and
  // the purpose catalogue, so it must not mount with the rail — and it must not
  // unmount on close either, because the drawer is still on screen while it
  // leaves and an emptied one is what the reader would watch go.
  const [everManaged, setEverManaged] = useState(false);
  const titleId = useId();
  const mayWrite = useCanWriteRecord("contact", view.contact);
  const entries = guard?.entries ?? [];
  // Correspondence is the one purpose the MAIL row speaks for: it is the
  // purpose a reply rides, and the only one an inbound message can flip on its
  // own. Where a workspace defines none the row stays unanswered, because a
  // newsletter's grant drawn against Email is a permission the composer then
  // refuses — two true answers a rep cannot reconcile. Every other purpose
  // carries its own row and its own name below.
  const correspondence = entries.find(
    (entry) => entry.purpose_class === "business_correspondence",
  );
  const otherPurposes = entries.filter(
    (entry) => entry.channel === "email" && entry !== correspondence,
  );
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
            {Object.entries(TRANSPORT_ROWS).map(([channel, transport]) => (
              <ConsentRow
                key={channel}
                {...transport({
                  contact: view.contact,
                  entries,
                  correspondence,
                  t,
                })}
              />
            ))}
            {/* A blocked identity still gets its row, with `reachable: false`: the
          conversation happened, and hiding the transport it happened on would
          answer "can I write to them" by pretending they were never here.
          Correspondence answers for these too, and that is not the borrowing
          the mail row refuses: the guard's channel is derived from the purpose
          class, so it separates mail-shaped purposes from phone and says
          nothing about chat — while a reply to someone who wrote to us rides
          the same lawful basis whichever transport carried it. */}
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
        <Button
          variant="ghost"
          onClick={() => {
            setEverManaged(true);
            setManage(true);
          }}
        >
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
      <Modal
        open={manage}
        onClose={() => setManage(false)}
        labelledBy={titleId}
        placement="right"
      >
        <div className="pe-drawer-title">
          <Heading size="large" id={titleId}>
            {t("consent.manage")}
          </Heading>
        </div>
        {everManaged && (
          <ConsentSection
            contactId={view.contact.id}
            contact={view.contact}
            showConfirm={false}
            titleLevel={3}
          />
        )}
      </Modal>
    </Panel>
  );
}

// One transport's row. Reachability is a fact about the RECORD and the verdict
// is a fact about consent, and the row states the first before the second: a
// permission to send where there is nowhere to send is not one a rep can act
// on, and colouring it green says they may.
type ConsentRowProps = Readonly<{
  icon: ReactNode;
  label: string;
  reachable: boolean;
  verdict: GuardEntry["verdict"] | undefined;
  // Why this purpose answers as it does, under the row it explains. Absent for
  // the transports, which answer on reachability and need no sentence.
  reason?: string;
  unreachableWord: string;
}>;

function ConsentRow({
  icon,
  label,
  reachable,
  verdict,
  reason,
  unreachableWord,
}: ConsentRowProps) {
  const t = useT();
  return (
    <>
      <div className="pe-rail-row">
        <span className="pe-rail-label">
          {icon}
          {label}
        </span>
        <span className={reachable ? verdictClass(verdict) : "pe-rail-value "}>
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

type TransportRow = (reading: TransportReading) => ConsentRowProps;

type TransportReading = Readonly<{
  contact: Contact360["contact"];
  entries: readonly GuardEntry[];
  correspondence: GuardEntry | undefined;
  t: Translator;
}>;

// One row per channel the guard answers on, keyed by the contract's union: a
// channel it gains — or drops — fails the build here, so no entry reaches the
// rail with nowhere to be drawn.
const TRANSPORT_ROWS: Record<Channel, TransportRow> = {
  email: ({ contact, correspondence, t }) => ({
    icon: <Mail size={15} aria-hidden="true" />,
    label: correspondence?.purpose_label ?? t("contact.rail.email"),
    reachable: (contact.emails?.length ?? 0) > 0,
    verdict: correspondence?.verdict,
    reason: correspondence?.reason,
    unreachableWord: t("contact.rail.noEmailAddress"),
  }),
  phone: ({ contact, entries, t }) => ({
    icon: <Phone size={15} aria-hidden="true" />,
    label: t("contact.rail.phone"),
    reachable: (contact.phones?.length ?? 0) > 0,
    verdict: entries.find((entry) => entry.channel === "phone")?.verdict,
    unreachableWord: t("contact.rail.noPhoneNumber"),
  }),
};

// Paired with consentWord's table, and with the same `??` for the same reason:
// the union is a claim about the wire, so a verdict off a newer server than
// this build would otherwise reach the class attribute as `undefined` and the
// row would carry no tone at all.
const VERDICT_TONE: Record<GuardEntry["verdict"], string> = {
  allowed: "pe-rail-value-good",
  blocked: "pe-rail-value-warning",
  unknown: "pe-rail-value-muted",
};

function verdictClass(verdict: GuardEntry["verdict"] | undefined): string {
  const tone = (verdict && VERDICT_TONE[verdict]) ?? VERDICT_TONE.unknown;
  return `pe-rail-value ${tone}`;
}
