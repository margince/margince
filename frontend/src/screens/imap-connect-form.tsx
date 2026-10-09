// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useId, useState } from "react";
import { api } from "../api/client";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { formatNumber, identifierNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { CaptureNotice } from "./capture-notice";
import { problemCodeOf, problemMessageOf, unwrap } from "./common";
import "./common.css";
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

type Range = { min: number; max: number };
const PORT_RANGE: Range = { min: 1, max: 65535 };
// The server clamps a larger count rather than refusing it; the form asks first.
const MAX_MESSAGES_RANGE: Range = { min: 1, max: 200 };

function inRange(value: number, { min, max }: Range): boolean {
  return Number.isInteger(value) && value >= min && value <= max;
}

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
  /**
   * Where the two buttons go. Inline under the fields by default; a surface
   * with a rail of its own (the first-run stage) places them there. Connect
   * calls the same submit Enter does, so it works wherever it is rendered.
   */
  renderActions?: (actions: ReactNode) => ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const formId = useId();
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
      return unwrap(
        await api.POST("/connectors/{provider}/connect", {
          params: { path: { provider: "imap" } },
          body: {
            imap: request,
          },
        }),
      );
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
  const portInRange = inRange(parsedPort, PORT_RANGE);
  const maxInRange = inRange(parsedMax, MAX_MESSAGES_RANGE);
  // Filled in but unusable: named like the missing fields, never a silent no-op.
  const outOfRange = [
    [!portInRange, t("connectors.imapPort")],
    [!maxInRange, t("connectors.imapMaxMessages")],
  ]
    .filter((bad): bad is [true, string] => bad[0] === true)
    .map(([, label]) => label);
  const ready = missing.length === 0 && outOfRange.length === 0;
  const needed = (absent: boolean) =>
    attempted && absent ? t("connectors.imapNeeded") : undefined;
  const ranged = (
    fits: boolean,
    range: Range,
    written: (bound: number) => string,
  ) =>
    attempted && !fits
      ? t("connectors.imapRange", {
          min: written(range.min),
          max: written(range.max),
        })
      : undefined;
  const refusal =
    missing.length > 0
      ? t("connectors.imapStillNeeded", { fields: missing.join(", ") })
      : t("connectors.imapOutOfRange", { fields: outOfRange.join(", ") });

  const errorMessage = connect.isError
    ? imapErrorMessage(connect.error, t)
    : null;

  // One submit for Enter in a field and for the Connect button, wherever the
  // button stands: pressable whatever is filled in, and the press is what
  // marks the missing fields and names them beside it.
  const submit = () => {
    // The button carries `pending`, but Enter in a field reaches this directly
    // and mutations run in parallel by default — so a second press while the
    // first is in flight would open a second IMAP session with the same
    // credentials. The guard belongs here, where both paths meet.
    if (connect.isPending) {
      return;
    }
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
      {attempted && !ready && <ErrorLine inline>{refusal}</ErrorLine>}
      <Button type="button" onClick={onDismiss} disabled={connect.isPending}>
        {dismissLabel}
      </Button>
      <Button
        variant="primary"
        type="submit"
        form={formId}
        pending={connect.isPending}
        busyLabel={t("create.saving")}
      >
        {t("connectors.imapSubmitCta")}
      </Button>
    </>
  );

  return (
    <>
      <form
        id={formId}
        className="form-stack"
        // The press names what is missing itself; a native bubble would pre-empt it.
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <div className="imap-mailbox-form">
          {/* Before the fields: read after typing a password, it comes too late. */}
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
          <Field
            label={t("connectors.imapPort")}
            error={ranged(portInRange, PORT_RANGE, identifierNumber)}
          >
            {(control) => (
              <TextInput
                {...control}
                type="number"
                min={PORT_RANGE.min}
                max={PORT_RANGE.max}
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
          <Field
            label={t("connectors.imapMaxMessages")}
            error={ranged(maxInRange, MAX_MESSAGES_RANGE, (count) =>
              formatNumber(count, locale),
            )}
          >
            {(control) => (
              <TextInput
                {...control}
                type="number"
                min={MAX_MESSAGES_RANGE.min}
                max={MAX_MESSAGES_RANGE.max}
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
              <ErrorLine>{errorMessage}</ErrorLine>
            </div>
          )}
        </div>
      </form>
      {renderActions(<div className="actions">{actions}</div>)}
    </>
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
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("connectors.imapModalTitle")}
      </Heading>
      {open && (
        <ImapMailboxForm
          dismissLabel={t("create.cancel")}
          onDismiss={onClose}
          onConnected={onConnected}
        />
      )}
    </Modal>
  );
}
