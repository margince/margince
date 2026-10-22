import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import {
  asOAuthConnectProvider,
  type OAuthConnectProvider,
  VENDOR_SERVICES,
  vendorOf,
} from "../../format/connectorprovider";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useConnectors } from "../../lib/connectors";
import {
  readStored,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../../lib/storage";
import { ReturnWarn } from "./onboarding-connect-panels.imap";

// The onboarding connect panels: what a consent round trip came back with, and
// the IMAP form, which lives in its own file. Neither claims a connection the
// server did not confirm: a live mailbox is shown by the step's account panel,
// from the roster.

export { ImapConnectPanel } from "./onboarding-connect-panels.imap";

// The OAuth outcomes that no retry can clear: the provider refused the grant,
// its API was never enabled for this deployment, or it refused this
// deployment's client credentials. Keyed off the server's outcome segment, and
// pointing at the same copy Settings renders so the two surfaces cannot drift
// apart.
const PERMANENT_FAILURE_BODY: Record<string, MessageKey | undefined> = {
  misconfigured: "connectors.oauthMisconfigured",
  bad_client: "connectors.oauthBadClient",
  rejected: "connectors.oauthRejected",
};

// The consent return carries its provider as a route segment, narrowed by
// asOAuthConnectProvider. null means "no provider this build knows", which is
// NOT the same fact as the segment being absent: the caller keeps the two apart.

// A real "allow" click leaves the page for the provider's consent screen. Only
// this tab's sessionStorage survives that round trip. It tells a real return
// apart from a stale or bookmarked `/connect/ok/...` URL replayed with no live
// attempt. `sessionStorage` (not `localStorage`) keeps the mark in the one tab
// that started the trip.
export function markOAuthAttempt(provider: OAuthConnectProvider): void {
  writeStored(STORAGE_KEYS.oauthAttempt, provider);
}

/** The provider a real attempt from THIS tab is returning for, or null if
 * none is recorded. Read-only, so the caller decides when the mark is
 * actually spent (`clearOAuthAttempt`). */
export function peekOAuthAttempt(): OAuthConnectProvider | null {
  return asOAuthConnectProvider(
    readStored(STORAGE_KEYS.oauthAttempt) ?? undefined,
  );
}

/** Spends the mark so a reload of the same return URL reads as the stale
 * link it now is, not a second live attempt. */
export function clearOAuthAttempt(): void {
  removeStored(STORAGE_KEYS.oauthAttempt);
}

/** Whether a roster row is a Google or Microsoft mailbox. A chained consent
 *  leaves a calendar row beside it, and the confirmation is the mailbox's. */
function isOAuthMailbox(provider: string): boolean {
  return Object.values(VENDOR_SERVICES).some((pair) => pair.mail === provider);
}

// What a consent round trip came back with, said only where the connect
// surface cannot. That is a refusal or a failure, in place of the account
// panel. A granted mailbox or calendar needs no word, because the roster is the
// proof. A mail-and-calendar press lands on the calendar's outcome, the last
// leg the server ran. The chained mailbox is already on the account panel.
export function OAuthReturnPanel({
  outcome,
  provider,
  onDone,
}: Readonly<{
  outcome?: string;
  /** The provider the consent returned for, from the deep-link route. */
  provider?: string;
  /** Closes the result once nothing confirmed a mailbox, so the reader is back
   * on the step's ask rather than reading a finished trip. */
  onDone: () => void;
}>) {
  const t = useT();
  const returning = asOAuthConnectProvider(provider);
  const vendor = vendorOf(returning ?? undefined);
  const mailbox = vendor === null ? null : VENDOR_SERVICES[vendor].mail;
  if (returning !== null && returning !== mailbox) {
    return outcome === "ok" ? null : (
      <ReturnWarn
        title={t("ob.conv.connect.calendarFailedTitle")}
        body={t(calendarFailureBody(outcome))}
        tone={outcome === "denied" ? "warning" : "danger"}
      />
    );
  }
  const failure = mailFailure(
    outcome,
    provider !== undefined && returning === null,
  );
  if (failure) {
    return (
      <ReturnWarn
        title={t(failure.title)}
        body={t(failure.body)}
        tone={outcome === "denied" ? "warning" : "danger"}
      />
    );
  }
  return <MailboxConfirm mailbox={mailbox} onDone={onDone} />;
}

/** A granted mail leg, until the roster confirms it. The exact mailbox is
 *  matched when the return named it, because the roster is provider-ordered
 *  and the first OAuth row could be a different vendor's. */
function MailboxConfirm({
  mailbox,
  onDone,
}: Readonly<{ mailbox: string | null; onDone: () => void }>) {
  const t = useT();
  const connections = useConnectors();
  const live = connections.data?.data.some(
    (c) =>
      c.status === "connected" &&
      (mailbox === null ? isOAuthMailbox(c.provider) : c.provider === mailbox),
  );
  if (connections.isPending) {
    return <p className="t-caption">{t("ob.s4.connectVerifying")}</p>;
  }
  // A live mailbox is the account panel's to show.
  if (live) {
    return null;
  }
  return (
    <Callout
      tone="danger"
      kind="outcome"
      title={t("ob.s4.connectConfirmFailed")}
      actions={
        // A roster that FAILED is not a fact in either direction, so the exit
        // is withheld and the surface's roster failure says what to do.
        !connections.isError && (
          <Button onClick={onDone}>{t("ob.conv.connect.dialogDone")}</Button>
        )
      }
    >
      <p>{t("ob.s4.connectRetry")}</p>
    </Callout>
  );
}

/** What a mail leg that did not land says, or null for one that did. Onboarding
 *  is the DEFAULT return surface, so it handles the whole outcome enum: the
 *  permanent failures reuse the Settings wording and never say "try again". */
function mailFailure(
  outcome: string | undefined,
  unresolvedProvider: boolean,
): { title: MessageKey; body: MessageKey } | null {
  if (outcome === "denied") {
    return { title: "ob.s4.connectDenied", body: "ob.s4.connectRetry" };
  }
  if (outcome !== "ok" || unresolvedProvider) {
    return {
      title: "ob.s4.connectConfirmFailed",
      body: permanentBody(outcome) ?? "ob.s4.connectRetry",
    };
  }
  return null;
}

function calendarFailureBody(outcome: string | undefined): MessageKey {
  if (outcome === "denied") {
    return "ob.conv.connect.calendarDeclined";
  }
  return permanentBody(outcome) ?? "ob.s4.connectRetry";
}

// Object.hasOwn, not a bare index: a route segment like "constructor" would
// otherwise resolve to an inherited member and render an empty banner.
function permanentBody(outcome: string | undefined): MessageKey | undefined {
  return outcome && Object.hasOwn(PERMANENT_FAILURE_BODY, outcome)
    ? PERMANENT_FAILURE_BODY[outcome]
    : undefined;
}
