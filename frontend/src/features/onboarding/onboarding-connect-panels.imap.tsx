import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  Circle,
  CircleCheck,
  Mail,
  ShieldCheck,
} from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "../../api/client";
import { Button, Disclosure, Field } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { CaptureNotice } from "../capture/capture-notice";
import { imapErrorMessage } from "../capture/imap-connect-form";
import { ConnectPostureStep } from "../settings/connect-posture";

// The failure banner the connect panels share.
export function ConnectWarn({ title, body }: { title: string; body: string }) {
  return (
    // The measure and the centring belong to the stylesheet. An inline `margin`
    // shorthand also resets the banner's top margin, so the failure block would
    // have nothing above it.
    <div className="readfail warning ob-connect-warning">
      <span className="rfi">
        <Circle aria-hidden />
      </span>
      <div>
        <div className="rft">{title}</div>
        {/* ds:ignore the body under the read-failure banner's title */}
        <p className="rfp">{body}</p>
      </div>
    </div>
  );
}

// IMAP is a standing connection. It mirrors the typed POST of the Settings form
// (imap-connect-form.tsx): the same `{imap:{...}}` shape and two error
// sentences. So onboarding and Settings agree on what "connect" means here.
// The connect call returns before any mail is read, so there is no capture
// count to show. Only a live row (last_synced_at) fills in once the sweep runs.
const IMAP_DEFAULT_PORT = "993";

// `onDismiss` closes the dialog without connecting. See `OAuthConnectPanel`
// for why that is a distinct action from skipping the whole required step.
export function ImapConnectPanel({
  onDone,
  onDismiss,
  onPendingChange,
}: Readonly<{
  /** Closes the panel once the mailbox is live; the step is left from the
   * connect surface, never from here (see `OAuthReturnPanel`). */
  onDone: () => void;
  onDismiss: () => void;
  /** See `OAuthConnectPanel`'s own `onPendingChange` for what this reports
   * and why the caller needs it. */
  onPendingChange?: (pending: boolean) => void;
}>) {
  const t = useT();
  const qc = useQueryClient();
  // This panel's own flag. The OAuth arm (`OAuthConnectPanel`) has a different
  // lifetime, and one shared flag would couple two flows that never run
  // together.
  const [postureSaving, setPostureSaving] = useState(false);
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
      return unwrap(
        await api.POST("/connectors/{provider}/connect", {
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
        }),
      );
    },
    onSuccess: () => {
      // The Settings connected-inboxes card shares this query key. A connect
      // made in onboarding must show there at once, not only on that card's
      // next mount.
      void qc.invalidateQueries({ queryKey: ["connectors"] });
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

  if (connect.data?.connection) {
    return (
      <div className="connect-result">
        <div className="cr-h">
          <CircleCheck aria-hidden /> {t("ob.s4.capturedTitle")}
        </div>
        <p className="ob-sub">{t("ob.s4.capturedBody")}</p>
        {/* The same question the OAuth arm asks, in the same place: after the
            grant, before the reader leaves for the CRM. The connect stores the
            credentials; the first window of mail is read afterwards by the
            standing sync, and the row is due the moment it exists. So this is
            the last screen on which the answer can still precede the mail. */}
        <ConnectPostureStep
          provider={connect.data.connection.provider}
          posture={connect.data.connection.mail_posture ?? undefined}
          onPendingChange={setPostureSaving}
        />
        <Button
          variant="primary"
          // Closing mid-write leaves the answer in flight with no surface left
          // to show a refusal.
          disabled={postureSaving}
          onClick={onDone}
        >
          {t("ob.conv.connect.dialogDone")} <ArrowRight aria-hidden />
        </Button>
      </div>
    );
  }

  return (
    <>
      <div className="form-stack">
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
          <ConnectWarn
            title={t("ob.s4.connectFailed")}
            body={imapErrorMessage(connect.error, t)}
          />
        )}

        {/* Same words as the OAuth panel, and in the same place: the last thing
          read before the mailbox is connected. */}
        <CaptureNotice />
      </div>

      <div className="actions">
        <Button
          // See `OAuthConnectPanel`'s own `onDismiss` for why the in-flight
          // POST has to finish before this becomes a real choice again.
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
