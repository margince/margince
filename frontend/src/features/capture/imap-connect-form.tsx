// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { CaptureNotice } from "../../components/capture-notice";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { useFieldFaults } from "../../design-system/savepage";
import { formatNumber, identifierNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import {
  IMAP_PORT_DEFAULT,
  imapErrorMessage,
  isWebAddress,
  PORT_RANGE,
  type Range,
  rangeFault,
  rangeParams,
} from "../../lib/imapconnect";
import {
  type FieldLabels,
  fieldCheckSentence,
  unwrap,
} from "../../lib/problem";
import {
  ImapSendFields,
  READ_ONLY,
  smtpPort,
  smtpRequest,
} from "./imap-send-fields";
import "../../design-system/querystates.css";
import "./imap-connect-form.css";

// The IMAP connect flavor (RC-8/Task 6): the credential providers' first-
// connect and reconnect both happen through this one form — there is no OAuth
// redirect to bounce through, so the standing connect (Task 1's `{imap:{...}}`
// shape) IS the whole act. The typed client only, hitting the same standing
// `/connectors/imap/connect` onboarding's ImapConnectPanel
// (onboarding-connect-panels.imap.tsx) posts to.
//
// Two surfaces render the form: Settings, in a dialog, and the first-run
// platform step, inline. ImapMailboxForm is the one form; ImapConnectForm is
// the dialog around it.

type ImapConnectRequest = components["schemas"]["ImapCredentials"];

const DEFAULT_PORT = String(IMAP_PORT_DEFAULT);
const DEFAULT_MAILBOX = "INBOX";
const DEFAULT_MAX_MESSAGES = "50";

// The server clamps a larger count rather than refusing it; the form asks first.
const MAX_MESSAGES_RANGE: Range = { min: 1, max: 200 };

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
  renderActions = (actions) => <div className="actions">{actions}</div>,
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
  const [send, setSend] = useState(READ_ONLY);
  const [faults, faultsRoot] = useFieldFaults();
  // The password is emptied after a refused connect, which is not a missing one.
  const [secretEmptied, setSecretEmptied] = useState(false);

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
    onSuccess: async () => {
      // Never claim a connection the server did not confirm: the row list
      // is the proof, so invalidate it and let the card's own re-read of
      // GET /connectors drive whatever it shows next.
      setSecret("");
      // Closed only once the roster has redrawn, so focus lands on what it drew.
      await queryClient.invalidateQueries({ queryKey: ["connectors"] });
      onConnected?.();
    },
    onError: () => {
      // The secret is never retained after a failed submit.
      setSecret("");
      setSecretEmptied(true);
    },
  });
  useEffect(() => {
    onPendingChange?.(connect.isPending);
    return () => onPendingChange?.(false);
  }, [connect.isPending, onPendingChange]);

  const parsedPort = port.trim() === "" ? IMAP_PORT_DEFAULT : Number(port);
  const parsedMax = maxMessages.trim() === "" ? 50 : Number(maxMessages);
  const whole = (wire: string) => identifierNumber(Number(wire));
  const labels: FieldLabels = {
    host: t("connectors.imapHost"),
    port: { label: t("connectors.imapPort"), limit: whole },
    username: t("connectors.imapUsername"),
    secret: t("connectors.imapSecret"),
    max_messages: {
      label: t("connectors.imapMaxMessages"),
      limit: (wire) => formatNumber(Number(wire), locale),
    },
    smtp_port: { label: t("connectors.imapSmtpPort"), limit: whole },
  };
  // Each field a press would refuse, in the words the server's refusal gets.
  const checked = () => {
    const found = new Map<string, string>();
    const refuse = (field: string, code: string, bounds?: Range) =>
      found.set(
        field,
        fieldCheckSentence(
          code,
          locale,
          labels[field],
          bounds && rangeParams(bounds),
        ),
      );
    const rangeOf = (field: string, value: number, bounds: Range) => {
      const fault = rangeFault(value, bounds);
      if (fault) refuse(field, fault, bounds);
    };
    if (host.trim() === "") refuse("host", "required");
    else if (isWebAddress(host))
      found.set("host", t("connectors.imapHostName"));
    if (username.trim() === "") refuse("username", "required");
    if (secret === "") refuse("secret", "required");
    rangeOf("port", parsedPort, PORT_RANGE);
    rangeOf("max_messages", parsedMax, MAX_MESSAGES_RANGE);
    if (send.on) rangeOf("smtp_port", smtpPort(send), PORT_RANGE);
    return found;
  };

  const errorMessage = connect.isError
    ? imapErrorMessage(connect.error, t)
    : null;
  // A new value ends the refusal of the old one, on its field and at the foot.
  const edit =
    (field: string, set: (value: string) => void) =>
    (event: { target: { value: string } }) => {
      set(event.target.value);
      faults.edited(field);
      connect.reset();
      if (field === "secret") setSecretEmptied(false);
    };

  // One submit for Enter in a field and for the Connect button, wherever the
  // button stands: pressable whatever is filled in, and the press marks the
  // fields it refuses.
  const submit = () => {
    // The button carries `pending`, but Enter in a field reaches this directly
    // and mutations run in parallel by default — so a second press while the
    // first is in flight would open a second IMAP session with the same
    // credentials. The guard belongs here, where both paths meet.
    if (connect.isPending) {
      return;
    }
    connect.reset();
    if (faults.show(checked())) {
      return;
    }
    connect.mutate({
      host: host.trim(),
      port: parsedPort,
      username: username.trim(),
      secret,
      mailbox: mailbox.trim() || DEFAULT_MAILBOX,
      max_messages: parsedMax,
      smtp: smtpRequest(send),
    });
  };

  const actions = (
    <>
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
        ref={faultsRoot}
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
            error={faults.of("host")}
          >
            {(control) => (
              <TextInput
                {...control}
                value={host}
                onChange={edit("host", setHost)}
              />
            )}
          </Field>
          <Field label={t("connectors.imapPort")} error={faults.of("port")}>
            {(control) => (
              <TextInput
                {...control}
                inputMode="numeric"
                value={port}
                onChange={edit("port", setPort)}
              />
            )}
          </Field>
          <Field
            label={t("connectors.imapUsername")}
            required
            error={faults.of("username")}
          >
            {(control) => (
              <TextInput
                {...control}
                type="email"
                autoComplete="email"
                value={username}
                onChange={edit("username", setUsername)}
              />
            )}
          </Field>
          <Field
            label={t("connectors.imapSecret")}
            required
            hint={
              secretEmptied && secret === ""
                ? t("connectors.imapSecretEmptied")
                : undefined
            }
            error={faults.of("secret")}
          >
            {(control) => (
              <TextInput
                {...control}
                type="password"
                autoComplete="off"
                value={secret}
                onChange={edit("secret", setSecret)}
              />
            )}
          </Field>
          <Field label={t("connectors.imapMailbox")}>
            {(control) => (
              <TextInput
                {...control}
                value={mailbox}
                onChange={edit("mailbox", setMailbox)}
              />
            )}
          </Field>
          <Field
            label={t("connectors.imapMaxMessages")}
            error={faults.of("max_messages")}
          >
            {(control) => (
              <TextInput
                {...control}
                inputMode="numeric"
                value={maxMessages}
                onChange={edit("max_messages", setMaxMessages)}
              />
            )}
          </Field>
          <ImapSendFields
            draft={send}
            onChange={(next) => {
              setSend(next);
              faults.edited("smtp_port");
            }}
            imapHost={host}
            portError={faults.of("smtp_port")}
          />
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
      {renderActions(actions)}
    </>
  );
}

/** The Settings dialog: ImapMailboxForm under a heading, mounted per open. */
export function ImapConnectForm({
  open,
  onClose,
  onConnected,
  returnFocusTo,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  onConnected?: () => void;
  returnFocusTo?: () => HTMLElement | null;
}>) {
  const t = useT();
  const headingId = useId();
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={headingId}
      intent="form"
      returnFocusTo={returnFocusTo}
    >
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
