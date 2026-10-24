import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Mail, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "../../api/client";
import { CaptureNotice } from "../../components/capture-notice";
import {
  Button,
  Disclosure,
  Field,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { imapErrorMessage } from "../capture/imap-connect-form";

/** A leg that did not land. A refusal the reader gave is theirs to answer
 *  again (amber); anything else broke (red). */
export function ReturnWarn({
  title,
  body,
  tone = "danger",
}: Readonly<{ title: string; body: string; tone?: "warning" | "danger" }>) {
  return (
    <Callout tone={tone} kind="outcome" title={title}>
      <p>{body}</p>
    </Callout>
  );
}

// IMAP: a standing connection, mirroring the typed POST of the Settings inline
// form (imap-connect-form.tsx). It uses the same nested `{imap:{...}}` shape
// and IMAP error sentences, so onboarding and Settings cannot drift apart.
// The connect call returns before any mail is read. There is no capture count
// to show, only a live row (last_synced_at) that fills in minutes later.
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
    // The mailbox is live once the shared roster says so. The step's account panel
    // and Settings read that key. The form closes only after it has been read
    // again, onto a step that already shows the mailbox.
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
            <TextInput
              {...control}
              value={host}
              placeholder={t("ob.s4.imapHostPlaceholder")}
              onChange={(e) => setHostVal(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("ob.s4.imapPort")}>
          {(control) => (
            <TextInput
              {...control}
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
            <TextInput
              {...control}
              type="email"
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          )}
        </Field>
        <Field className="full" label={t("ob.s4.imapPassword")}>
          {(control) => (
            <TextInput
              {...control}
              type="password"
              autoComplete="off"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("ob.s4.imapMailbox")}>
          {(control) => (
            <TextInput
              {...control}
              value={mailbox}
              onChange={(e) => setMailbox(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("ob.s4.imapMax")}>
          {(control) => (
            <TextInput
              {...control}
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
          // Dismissal is a decision not to connect, so it waits for the POST it would
          // abandon: a success landing after it contradicts it.
          disabled={connect.isPending}
          onClick={onDismiss}
        >
          {t("ob.s4.notNow")}
        </Button>
        <Button
          variant="primary"
          pending={connect.isPending}
          disabled={!ready}
          onClick={() => connect.mutate()}
        >
          <Mail aria-hidden /> {t("ob.s4.imapConnect")}
        </Button>
      </div>
    </>
  );
}
