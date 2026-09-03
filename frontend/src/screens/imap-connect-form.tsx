// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useId, useState } from "react";
import { api } from "../api/client";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { CaptureNotice } from "./capture-notice";
import { problemCodeOf, problemMessageOf, throwProblem } from "./common";
import "./imap-connect-form.css";

// The IMAP connect flavor (RC-8/Task 6): the credential providers' first-
// connect and reconnect both happen through this one form — there is no OAuth
// redirect to bounce through, so the standing connect (Task 1's `{imap:{...}}`
// shape) IS the whole act. The typed client only, hitting the same standing
// `/connectors/imap/connect` onboarding's ImapConnectPanel
// (onboarding-connect-panels.tsx) posts to.
//
// Two surfaces render the form: Settings, in a dialog, and the first-run
// platform step, inline. ImapMailboxForm is the one form; ImapConnectForm is
// the dialog around it.

type ImapConnectRequest = {
  host: string;
  port: number;
  username: string;
  secret: string;
  mailbox: string;
  max_messages: number;
};

const DEFAULT_PORT = "993";
const DEFAULT_MAILBOX = "INBOX";
const DEFAULT_MAX_MESSAGES = "50";

// The two IMAP-specific server conditions get their own honest sentence;
// every other failure reads the way failures read everywhere else. Neither
// sentence ever echoes the submitted host — the server doesn't send it back
// either, so there is nothing here to leak. Exported so the onboarding IMAP
// panel (onboarding-connect-panels.tsx) reads the same two sentences off the
// same server codes, rather than growing its own copy of this mapping.
export function imapErrorMessage(
  error: unknown,
  t: (key: MessageKey) => string,
): string {
  const code = problemCodeOf(error);
  if (code === "imap_login_rejected") {
    return t("connectors.imapLoginRejected");
  }
  if (code === "imap_unreachable") {
    return t("connectors.imapUnreachable");
  }
  return problemMessageOf(error, t);
}

/**
 * The mailbox form itself: fields, the standing connect, and the two actions.
 *
 * Mounted fresh by each surface: a dialog mounts it on open, so no attempt's
 * values — least of all the secret — survive into the next one.
 */
export function ImapMailboxForm({
  dismissLabel,
  onDismiss,
  onConnected,
  onPendingChange,
  small = false,
  renderActions = (actions) => actions,
}: Readonly<{
  /** What backing out is called on this surface: Cancel in a dialog, Not
   * now on a step that does not block. */
  dismissLabel: string;
  onDismiss: () => void;
  // Called after the server has confirmed the connection — never before.
  // The caller's own row list (GET /connectors, invalidated below) is what
  // actually proves it; this callback just closes the caller's affordance.
  onConnected?: () => void;
  /** Reports the in-flight connect, so a surface that owns other controls
   * can hold them while the credentials are being proven. */
  onPendingChange?: (pending: boolean) => void;
  /** The dialog's compact buttons; a step in a room keeps the room's size. */
  small?: boolean;
  /**
   * Where the two buttons go. Inline under the fields by default; a surface
   * with a rail of its own (the first-run stage) places them there. Connect
   * calls the same submit Enter does, so it works wherever it is rendered.
   */
  renderActions?: (actions: ReactNode) => ReactNode;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [host, setHost] = useState("");
  const [port, setPort] = useState(DEFAULT_PORT);
  const [username, setUsername] = useState("");
  const [secret, setSecret] = useState("");
  const [mailbox, setMailbox] = useState(DEFAULT_MAILBOX);
  const [maxMessages, setMaxMessages] = useState(DEFAULT_MAX_MESSAGES);
  // Whether Connect has been pressed with something still missing. The button
  // is always pressable; the press is what turns the missing fields red and
  // names them beside it, so a reader learns what is needed by trying rather
  // than by guessing why a button is grey.
  const [attempted, setAttempted] = useState(false);

  const connect = useMutation({
    mutationFn: async (request: ImapConnectRequest) => {
      const { data, error } = await api.POST("/connectors/{provider}/connect", {
        params: { path: { provider: "imap" } },
        body: {
          imap: request,
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      // Never claim a connection the server did not confirm: the row list
      // is the proof, so invalidate it and let the card's own re-read of
      // GET /connectors drive whatever it shows next.
      queryClient.invalidateQueries({ queryKey: ["connectors"] });
      setSecret("");
      onConnected?.();
    },
    onError: () => {
      // The secret is never retained after a failed submit.
      setSecret("");
    },
  });
  useEffect(() => {
    onPendingChange?.(connect.isPending);
    return () => onPendingChange?.(false);
  }, [connect.isPending, onPendingChange]);

  const parsedPort = port.trim() === "" ? 993 : Number(port);
  const parsedMax = maxMessages.trim() === "" ? 50 : Number(maxMessages);
  // The fields without which nothing can be dialled, by label, in the order
  // they stand on the form: what the rail names once Connect is pressed.
  const missing = [
    [host.trim() === "", t("connectors.imapHost")],
    [username.trim() === "", t("connectors.imapUsername")],
    [secret === "", t("connectors.imapSecret")],
  ]
    .filter((need): need is [true, string] => need[0] === true)
    .map(([, label]) => label);
  const ready =
    missing.length === 0 &&
    Number.isInteger(parsedPort) &&
    parsedPort >= 1 &&
    parsedPort <= 65535 &&
    Number.isInteger(parsedMax) &&
    parsedMax >= 1 &&
    parsedMax <= 200;
  const needed = (absent: boolean) =>
    attempted && absent ? t("connectors.imapNeeded") : undefined;

  const errorMessage = connect.isError
    ? imapErrorMessage(connect.error, t)
    : null;

  // One submit for Enter in a field and for the Connect button, wherever the
  // button stands: pressable whatever is filled in, and the press is what
  // marks the missing fields and names them beside it.
  const submit = () => {
    if (!ready) {
      setAttempted(true);
      return;
    }
    connect.mutate({
      host: host.trim(),
      port: parsedPort,
      username: username.trim(),
      secret,
      mailbox: mailbox.trim() || DEFAULT_MAILBOX,
      max_messages: parsedMax,
    });
  };

  const actions = (
    <>
      {attempted && missing.length > 0 && (
        <p className="ob-stage-note" role="alert">
          {t("connectors.imapStillNeeded", { fields: missing.join(", ") })}
        </p>
      )}
      <Button
        small={small}
        type="button"
        onClick={onDismiss}
        disabled={connect.isPending}
      >
        {dismissLabel}
      </Button>
      <Button
        small={small}
        variant="primary"
        type="button"
        onClick={submit}
        pending={connect.isPending}
        busyLabel={t("create.saving")}
      >
        {t("connectors.imapSubmitCta")}
      </Button>
    </>
  );

  return (
    <form
      className="imap-mailbox-form"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      {/* Before the fields, not after: a person connecting a mailbox from
          Settings is told the same thing onboarding tells them, and reading
          it after typing a password is reading it too late. */}
      <div className="imap-mailbox-span">
        <CaptureNotice />
      </div>
      <Field
        label={t("connectors.imapHost")}
        required
        error={needed(host.trim() === "")}
      >
        {(control) => (
          <TextInput
            {...control}
            value={host}
            onChange={(event) => setHost(event.target.value)}
          />
        )}
      </Field>
      <Field label={t("connectors.imapPort")}>
        {(control) => (
          <TextInput
            {...control}
            type="number"
            min={1}
            max={65535}
            value={port}
            onChange={(event) => setPort(event.target.value)}
          />
        )}
      </Field>
      <Field
        label={t("connectors.imapUsername")}
        required
        error={needed(username.trim() === "")}
      >
        {(control) => (
          <TextInput
            {...control}
            type="email"
            autoComplete="email"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
        )}
      </Field>
      <Field
        label={t("connectors.imapSecret")}
        required
        error={needed(secret === "")}
      >
        {(control) => (
          <TextInput
            {...control}
            type="password"
            autoComplete="off"
            value={secret}
            onChange={(event) => setSecret(event.target.value)}
          />
        )}
      </Field>
      <Field label={t("connectors.imapMailbox")}>
        {(control) => (
          <TextInput
            {...control}
            value={mailbox}
            onChange={(event) => setMailbox(event.target.value)}
          />
        )}
      </Field>
      <Field label={t("connectors.imapMaxMessages")}>
        {(control) => (
          <TextInput
            {...control}
            type="number"
            min={1}
            max={200}
            value={maxMessages}
            onChange={(event) => setMaxMessages(event.target.value)}
          />
        )}
      </Field>
      <p className="t-caption imap-mailbox-span">
        {t("connectors.imapSecretHint")}
      </p>
      {errorMessage && (
        <div className="imap-mailbox-span">
          <Callout tone="danger" live="alert">
            {errorMessage}
          </Callout>
        </div>
      )}
      {renderActions(
        <div className="actions imap-mailbox-span">{actions}</div>,
      )}
    </form>
  );
}

/** The Settings dialog: ImapMailboxForm under a heading, mounted per open. */
export function ImapConnectForm({
  open,
  onClose,
  onConnected,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  onConnected?: () => void;
}>) {
  const t = useT();
  const headingId = useId();
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId}>
      <h2
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {t("connectors.imapModalTitle")}
      </h2>
      {open && (
        <ImapMailboxForm
          small
          dismissLabel={t("create.cancel")}
          onDismiss={onClose}
          onConnected={onConnected}
        />
      )}
    </Modal>
  );
}
