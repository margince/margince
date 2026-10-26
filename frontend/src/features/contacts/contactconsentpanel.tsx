import { useQuery } from "@tanstack/react-query";
import { Mail } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Modal, Skeleton } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import type { DetailRow } from "../../design-system/recorddetails";
import { type Translator, useLocale, useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import { useProviderLabel } from "../../lib/channelproviders";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { ConfirmDetailsAction, ConsentSection } from "../privacy/consent";
import { consentPurposeName, consentReason } from "./consentreason";
import { consentWord } from "./contactreadings";

type Contact360 = components["schemas"]["Contact360"];
type ContactConsentGuard = components["schemas"]["ContactConsentGuard"];
type GuardEntry = components["schemas"]["ContactConsentGuardEntry"];
type Channel = GuardEntry["channel"];

/** The guard's verdict per purpose, read once per contact: the left column
 *  and the Research tab's consent panel share the cached answer. The server
 *  refuses it for an archived row, so `enabled` waits for the row. */
export function useConsentGuard(contactId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["contactConsentGuard", contactId],
    enabled,
    queryFn: async (): Promise<ContactConsentGuard> =>
      unwrap(
        await api.GET("/contacts/{id}/consent/guard", {
          params: { path: { id: contactId } },
        }),
      ),
  });
}

/** The guard's answer as the left column reads it. */
export type ConsentRead = Readonly<{
  guard: ContactConsentGuard | undefined;
  loading: boolean;
  failed: boolean;
  onRetry: () => void;
}>;

// Correspondence is the one purpose the mail row speaks for. A reply rides
// it, and it is the only one an inbound message can flip on its own.
function correspondenceOf(entries: readonly GuardEntry[]) {
  return entries.find(
    (entry) => entry.purpose_class === "business_correspondence",
  );
}

/**
 * The left column's "Consent & channels" rows: one per transport the guard
 * answers on, then each chat identity the contact holds. Reachability is a
 * fact about the record and the verdict a fact about consent. A row says the
 * first before the second. Permission to send with nowhere to send is not one
 * a rep can act on.
 */
export function useConsentRows(
  view: Contact360,
  consent: ConsentRead,
): DetailRow[] {
  const t = useT();
  const providerLabel = useProviderLabel();
  const entries = consent.guard?.entries ?? [];
  const correspondence = correspondenceOf(entries);
  if (consent.loading) {
    return [
      { key: "consent-loading", term: <Skeleton width="4rem" />, value: null },
    ];
  }
  if (consent.failed) {
    return [
      {
        key: "consent-failed",
        term: t("contact.rail.consentTitle"),
        value: (
          <span role="alert">
            {t("consent.guardFailed")}{" "}
            <Button variant="link" onClick={consent.onRetry}>
              {t("common.retry")}
            </Button>
          </span>
        ),
        block: true,
      },
    ];
  }
  const transports = Object.entries(TRANSPORT_ROWS).map(([channel, row]) => ({
    key: channel,
    ...row({ contact: view.contact, entries, correspondence, t }),
  }));
  // A blocked identity keeps its row with `reachable: false`: the conversation
  // happened, and hiding its transport would pretend they were never here.
  const chats = (view.contact.reachability ?? []).map((channel) => ({
    key: `chat-${channel.provider}`,
    label: providerLabel(channel.provider),
    reachable: channel.reachable,
    verdict: correspondence?.verdict,
    unreachableWord: t("contact.rail.channelNotDeliverable"),
  }));
  return [...transports, ...chats].map((row) => ({
    key: row.key,
    term: row.label,
    value: (
      <span className={row.reachable ? verdictClass(row.verdict) : undefined}>
        {row.reachable ? consentWord(row.verdict, t) : row.unreachableWord}
      </span>
    ),
  }));
}

type TransportReading = Readonly<{
  contact: Contact360["contact"];
  entries: readonly GuardEntry[];
  correspondence: GuardEntry | undefined;
  t: Translator;
}>;

type TransportRow = Readonly<{
  label: string;
  reachable: boolean;
  verdict: GuardEntry["verdict"] | undefined;
  unreachableWord: string;
}>;

// One row per channel the guard answers on, keyed by the contract's union.
// A channel it gains or drops fails the build here, so no entry reaches the
// column with nowhere to be drawn.
const TRANSPORT_ROWS: Record<
  Channel,
  (reading: TransportReading) => TransportRow
> = {
  email: ({ contact, correspondence, t }) => ({
    label: t("contact.consent.businessMail"),
    reachable: (contact.emails?.length ?? 0) > 0,
    verdict: correspondence?.verdict,
    unreachableWord: t("contact.rail.noEmailAddress"),
  }),
  phone: ({ contact, entries, t }) => ({
    label: t("contact.rail.phone"),
    reachable: (contact.phones?.length ?? 0) > 0,
    verdict: entries.find((entry) => entry.channel === "phone")?.verdict,
    unreachableWord: t("contact.rail.noPhoneNumber"),
  }),
};

// Paired with consentWord's table, with the same fallback. A verdict off a
// newer server than this build reads as unknown, not as no tone at all.
const VERDICT_TONE: Record<GuardEntry["verdict"], string> = {
  allowed: "pe-detail-good",
  blocked: "pe-detail-warn",
  unknown: "",
};

function verdictClass(verdict: GuardEntry["verdict"] | undefined): string {
  return (verdict && VERDICT_TONE[verdict]) || "";
}

/**
 * The Research tab's consent panel. It lists every purpose other than
 * correspondence by name, each with the reason it answers as it does. Then
 * come the recorded consent and proof behind them, and the request that asks
 * the contact to confirm their details. A rep refused at the composer finds here which
 * purpose refused them, and why.
 */
export function ContactConsentPanel({ view }: Readonly<{ view: Contact360 }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const guard = useConsentGuard(view.contact.id, !view.contact.archived_at);
  const [manage, setManage] = useState(false);
  // Opened once, mounted from then on. The section reads consent and the
  // purpose catalogue, and a reader would watch an emptied drawer leave on
  // close.
  const [everManaged, setEverManaged] = useState(false);
  const titleId = useId();
  const mayWrite = useCanWriteRecord("contact", view.contact);
  const entries = guard.data?.entries ?? [];
  const correspondence = correspondenceOf(entries);
  const emails = view.contact.emails ?? [];
  const hasEmail = emails.length > 0;
  const knownRecipient =
    emails.length === 1 || emails.some((email) => email.is_primary);
  const purposes = entries.filter(
    (entry) => entry.channel === "email" && entry !== correspondence,
  );
  return (
    <Panel title={t("contact.rail.consentTitle")}>
      <PanelBody>
        {guard.isPending && <Skeleton width="100%" />}
        {hasEmail &&
          [correspondence, ...purposes]
            .filter((entry): entry is GuardEntry => entry !== undefined)
            .map((entry) => (
              <div key={entry.purpose_key}>
                <div className="pe-rail-row">
                  <span className="pe-rail-label">
                    <Mail size={15} aria-hidden="true" />
                    {consentPurposeName(entry, t)}
                  </span>
                  <span className={verdictClass(entry.verdict)}>
                    {consentWord(entry.verdict, t)}
                  </span>
                </div>
                {entry.reason && (
                  <p className="pe-colleague-proof t-caption">
                    {consentReason(entry, t, locale, zone)}
                  </p>
                )}
              </div>
            ))}
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
        intent="drawer"
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
