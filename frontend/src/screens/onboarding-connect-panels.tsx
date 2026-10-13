import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Mail, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import {
  readStored,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../app/storage";
import { Button, Disclosure, Field } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { CaptureNotice } from "./capture-notice";
import { throwProblem } from "./common";
import {
  asOAuthConnectProvider,
  type OAuthConnectProvider,
  VENDOR_SERVICES,
  vendorOf,
} from "./connectorproviders";
import { useConnectors } from "./connectors";
import { imapErrorMessage } from "./imap-connect-form";

// The onboarding connect panels: what a consent round trip came back with, and
// the IMAP form. Neither claims a connection the server did not confirm: a
// live mailbox is shown by the step's account panel, from the roster.

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

// A real "allow" click leaves the page entirely for the provider's own
// consent screen, and this tab's sessionStorage is the only thing that survives
// that round trip, so it is what tells a genuine return apart from a stale
// or bookmarked `/connect/ok/...` URL replayed with no live attempt behind
// it. `sessionStorage` (not `localStorage`) is deliberate: the mark belongs
// to the ONE tab that started the trip, the same scope the redirect itself
// stays within.
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
// surface cannot say it: a refusal or a failure, in place of the account panel.
// A granted mailbox and calendar need no word here, because the roster the
// surface draws its account panel from is the proof, never a static claim.
//
// A mail-and-calendar press lands on the calendar's own outcome, because the
// calendar consent is the last leg the server ran; the mailbox it was chained
// after is already on the account panel either way.
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
      />
    );
  }
  const failure = mailFailure(
    outcome,
    provider !== undefined && returning === null,
  );
  if (failure) {
    return <ReturnWarn title={t(failure.title)} body={t(failure.body)} />;
  }
  return <MailboxConfirm mailbox={mailbox} onDone={onDone} />;
}

function ReturnWarn({
  title,
  body,
}: Readonly<{ title: string; body: string }>) {
  return (
    <Callout tone="warning" kind="outcome" title={title}>
      <p>{body}</p>
    </Callout>
  );
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
      tone="warning"
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

// IMAP: a standing connection, mirroring the Settings inline form's typed
// POST (imap-connect-form.tsx) — the same nested `{imap:{...}}` shape and the
// same two IMAP-specific error sentences, so onboarding and Settings can
// never drift onto two different ideas of what "connect" means for this
// provider. The connect call returns BEFORE any mail is read: there is no
// capture count to show here, honestly — only a live row (last_synced_at)
// that fills in a few minutes later, once the sweep runs.
const IMAP_DEFAULT_PORT = "993";

// `onDismiss` puts the form away without connecting: the reader changed their
// mind about this provider, not about the step, which the surface skips.
export function ImapConnectPanel({
  onDone,
  onDismiss,
  onPendingChange,
}: Readonly<{
  /** Closes the panel once the mailbox is live; the step is left from the
   * connect surface, never from here (see `OAuthReturnPanel`). */
  onDone: () => void;
  onDismiss: () => void;
  /** Told whenever the connect POST's in-flight state changes, so the caller
   * can hold its own ways off this form (another vendor, the skip) too. */
  onPendingChange?: (pending: boolean) => void;
}>) {
  const t = useT();
  const qc = useQueryClient();
  const [host, setHostVal] = useState("imap.gmail.com");
  const [port, setPort] = useState(IMAP_DEFAULT_PORT);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [mailbox, setMailbox] = useState("INBOX");
  const [max, setMax] = useState("30");

  const parsedPort =
    port.trim() === "" ? Number(IMAP_DEFAULT_PORT) : Number(port);

  const connect = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST("/connectors/{provider}/connect", {
        params: { path: { provider: "imap" } },
        body: {
          imap: {
            host: host.trim(),
            port: parsedPort,
            username: email.trim(),
            secret: password,
            mailbox: mailbox.trim() || "INBOX",
            max_messages: Number(max) || 30,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // The mailbox is live once the shared roster says so: the step's account
    // panel (and Settings) read that key, so the form closes only after it
    // has been read again, onto a step that already shows the mailbox.
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["connectors"] });
      onDone();
    },
    onError: () => {
      // The secret is never retained after a failed submit, matching the
      // Settings form's practice.
      setPassword("");
    },
  });
  useEffect(() => {
    onPendingChange?.(connect.isPending);
  }, [connect.isPending, onPendingChange]);

  const parsedMax = max.trim() === "" ? 30 : Number(max);
  const ready =
    host.trim() !== "" &&
    Number.isInteger(parsedPort) &&
    parsedPort >= 1 &&
    parsedPort <= 65535 &&
    email.trim() !== "" &&
    password !== "" &&
    Number.isInteger(parsedMax) &&
    parsedMax >= 1 &&
    parsedMax <= 200;

  return (
    <>
      <div className="imap-form">
        <Field label={t("ob.s4.imapHost")}>
          {(control) => (
            <input
              {...control}
              className="input"
              value={host}
              placeholder={t("ob.s4.imapHostPlaceholder")}
              onChange={(e) => setHostVal(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("ob.s4.imapPort")}>
          {(control) => (
            <input
              {...control}
              className="input"
              type="number"
              min={1}
              max={65535}
              value={port}
              onChange={(e) => setPort(e.target.value)}
            />
          )}
        </Field>
        <Field className="full" label={t("ob.s4.imapEmail")}>
          {(control) => (
            <input
              {...control}
              className="input"
              type="email"
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          )}
        </Field>
        <Field className="full" label={t("ob.s4.imapPassword")}>
          {(control) => (
            <input
              {...control}
              className="input"
              type="password"
              autoComplete="off"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("ob.s4.imapMailbox")}>
          {(control) => (
            <input
              {...control}
              className="input"
              value={mailbox}
              onChange={(e) => setMailbox(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("ob.s4.imapMax")}>
          {(control) => (
            <input
              {...control}
              className="input"
              type="number"
              min={1}
              max={200}
              value={max}
              onChange={(e) => setMax(e.target.value)}
            />
          )}
        </Field>
      </div>

      <Disclosure summary={t("ob.s4.accessToggle")}>
        <p className="spoken-hint t-caption">
          <ShieldCheck aria-hidden /> {t("ob.s4.imapHint")}
        </p>
      </Disclosure>

      {connect.isError && (
        <ReturnWarn
          title={t("ob.s4.connectFailed")}
          body={imapErrorMessage(connect.error, t)}
        />
      )}

      {/* Same words as the OAuth panel, and in the same place: the last thing
          read before the mailbox is connected. */}
      <CaptureNotice />

      <div className="ob-connect-actions">
        <Button
          variant="ghost"
          // Dismissal is a decision NOT to connect, so it waits for the POST
          // it would abandon: a success landing after it contradicts it.
          disabled={connect.isPending}
          onClick={onDismiss}
        >
          {t("ob.s4.notNow")}
        </Button>
        <Button
          variant="primary"
          disabled={!ready || connect.isPending}
          onClick={() => connect.mutate()}
        >
          {connect.isPending ? (
            <>
              <span className="ob-spinner" /> {t("ob.s4.connecting")}
            </>
          ) : (
            <>
              <Mail aria-hidden /> {t("ob.s4.imapConnect")}
            </>
          )}
        </Button>
      </div>
    </>
  );
}
