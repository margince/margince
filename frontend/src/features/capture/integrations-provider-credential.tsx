// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plug } from "lucide-react";
import { useRef, useState } from "react";
import { api } from "../../api/client";
import {
  Badge,
  Button,
  Field,
  OverflowMenu,
  TextInput,
} from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ConfirmPhraseBox } from "../../design-system/confirmphrase";
import { Heading } from "../../design-system/heading";
import {
  ProviderMark,
  providerBrandName,
} from "../../design-system/provider-mark";
import { SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { connectionLabel, connectionTone } from "../../lib/provider-status";
import { recordWriteKeys } from "../../lib/recordwritekeys";
import type { ProviderConnection } from "./integrations-provider";

/** The provider's display name: its brand where this frontend knows one. */
function providerName(connection: ProviderConnection): string {
  return providerBrandName(connection.provider) ?? connection.provider;
}

// A heading, so heading navigation steps from one provider to the next. A row's
// label is a span, so the role says what the element cannot.
function ProviderIdentity({
  connection,
}: Readonly<{ connection: ProviderConnection }>) {
  const t = useT();
  return (
    <span className="provider-head">
      <span className="provider-mark">
        <ProviderMark providerKey={connection.provider} />
      </span>
      <Heading
        as="span"
        size="xsmall"
        role="heading"
        aria-level={3}
        className="provider-name"
      >
        {providerName(connection)}
      </Heading>
      <Badge tone={connectionTone(connection.status)}>
        {t(connectionLabel(connection.status))}
      </Badge>
    </span>
  );
}

// The two destructive decisions, behind the overflow they share. A component,
// not duplicated JSX: they render beside the key verb for a reader who may also
// connect, and alone for one who may not.
//
// The overflow sets their weight; tidiness is not the reason. Disconnect is
// recoverable (reconnect the key and lookups resume). Delete-data irreversibly
// destroys contact details this installation paid for. Neither belongs at the
// same weight as Connect, as the confirm ladder behind them (a plain confirm,
// then a typed one) already says.
function DestructiveActions({
  onDisconnect,
  onDeleteData,
}: Readonly<{ onDisconnect: () => void; onDeleteData: () => void }>) {
  const t = useT();
  return (
    <OverflowMenu label={t("record.moreActions")}>
      <Button type="button" onClick={onDisconnect}>
        {t("provider.disconnect")}
      </Button>
      <Button variant="danger" type="button" onClick={onDeleteData}>
        {t("provider.deleteData")}
      </Button>
    </OverflowMenu>
  );
}

// The key, and the confirm it is submitted through. Mounted only while open. A
// key sealed server-side is never returned. Holding a typed one after an
// abandoned dialog would keep a secret in the page for no purpose.
//
// The field lives inside the confirm rather than behind a second dialog on top
// of it. The posture is the same either way: nothing is sent until the confirm
// is pressed. This way the sentence about what connecting costs is on screen
// while the key is being pasted.
function CredentialDialog({
  connection,
  pending,
  error,
  onClose,
  onConnect,
}: Readonly<{
  connection: ProviderConnection;
  pending: boolean;
  error: string | null;
  onClose: () => void;
  onConnect: (apiKey: string) => void;
}>) {
  const t = useT();
  const [key, setKey] = useState("");
  const connected = connection.credential_present;
  return (
    <ConfirmModal
      open
      title={t("provider.connectConfirm.title")}
      confirmLabel={connected ? t("provider.reconnect") : t("provider.connect")}
      confirmDisabled={key.trim() === ""}
      onConfirm={() => onConnect(key.trim())}
      onClose={onClose}
      pending={pending}
      error={error}
    >
      <p>{t("provider.connectConfirm.body")}</p>
      {/* The field is write-only in both states: a sealed key is never sent
          back to the browser, so the box is empty even when one is in place.
          Left unexplained that reads as "no key connected" while the card
          behind it shows a live balance — so the label and the hint say which
          state this is, and the placeholder does not pretend to hold a
          value. */}
      <Field
        label={connected ? t("provider.apiKeyStored") : t("provider.apiKey")}
        hint={
          connected ? t("provider.apiKeyReplaceHint") : t("provider.apiKeyHint")
        }
      >
        {(control) => (
          <TextInput
            {...control}
            type="password"
            autoComplete="off"
            value={key}
            required
            placeholder={
              connected ? t("provider.apiKeyReplacePlaceholder") : ""
            }
            onChange={(event) => setKey(event.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}

export function CredentialRow({
  connection,
  canConnect,
  canDestroy,
}: Readonly<{
  connection: ProviderConnection;
  canConnect: boolean;
  canDestroy: boolean;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [connecting, setConnecting] = useState(false);
  const [disconnecting, setDisconnecting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [typed, setTyped] = useState("");
  const phraseInput = useRef<HTMLInputElement>(null);

  const connect = useMutation({
    // The key arrives as a variable rather than through a closure over the
    // dialog's state. The press belongs to the committed render, so what it
    // carries cannot be older than the field the reader typed into.
    mutationFn: async (apiKey: string) => {
      unwrap(
        await api.PUT("/provider-connections/{provider}", {
          params: { path: { provider: connection.provider } },
          body: { api_key: apiKey },
        }),
      );
    },
    onSuccess: () => {
      // Closing unmounts the dialog, which discards the typed key. The key is
      // sealed server-side and never returned, so keeping it would hold a
      // secret in the page for no purpose.
      setConnecting(false);
      void queryClient.invalidateQueries({
        queryKey: ["provider-connections"],
      });
    },
  });

  const disconnect = useMutation({
    mutationFn: async () => {
      unwrap(
        await api.DELETE("/provider-connections/{provider}", {
          params: { path: { provider: connection.provider } },
        }),
      );
    },
    onSuccess: () => {
      setDisconnecting(false);
      void queryClient.invalidateQueries({
        queryKey: ["provider-connections"],
      });
    },
  });

  const deleteData = useMutation({
    mutationFn: async () => {
      unwrap(
        await api.DELETE("/provider-connections/{provider}/data", {
          params: { path: { provider: connection.provider } },
        }),
      );
    },
    onSuccess: () => {
      setDeleting(false);
      setTyped("");
      // The deletion takes values off every contact the purchases filled, and
      // which contacts those are is the server's answer. So every contact read
      // is invalidated by the prefix of each per-record key, or the marks
      // outlive it.
      const contactReads = recordWriteKeys("contact", "").map(([prefix]) => [
        prefix,
      ]);
      for (const queryKey of [
        ["provider-connections"],
        ...contactReads,
        ["contactEmployments"],
      ]) {
        void queryClient.invalidateQueries({ queryKey });
      }
    },
  });

  const connected = connection.credential_present;
  // A stored key is what makes either destructive action meaningful, and the
  // grant is what makes it permitted.
  const destructive = connected && canDestroy;
  return (
    <>
      <SettingRow
        label={<ProviderIdentity connection={connection} />}
        description={
          connected ? t("provider.apiKeyReplaceHint") : t("provider.apiKeyHint")
        }
        control={
          <>
            {/* The key field goes behind the verb it feeds: two of the three
                things a connect needs — the key and the confirmation that it
                costs money — are not answers that fit in a row's right
                column. A reader who may not connect has nothing to open. */}
            {canConnect && (
              <Button
                variant="primary"
                type="button"
                onClick={() => setConnecting(true)}
              >
                <Plug aria-hidden />{" "}
                {connected ? t("provider.reconnect") : t("provider.connect")}
              </Button>
            )}
            {/* Stopping the flow and destroying what was bought are a
                different authority from connecting, so they stand on their own
                for a reader who holds one grant and not the other. */}
            {destructive && (
              <DestructiveActions
                onDisconnect={() => setDisconnecting(true)}
                onDeleteData={() => setDeleting(true)}
              />
            )}
          </>
        }
      />

      {connecting && (
        <CredentialDialog
          connection={connection}
          pending={connect.isPending}
          error={connect.isError ? problemMessageOf(connect.error, t) : null}
          onClose={() => setConnecting(false)}
          onConnect={(apiKey) => {
            if (!canConnect) {
              return;
            }
            connect.mutate(apiKey);
          }}
        />
      )}

      {/* Both destructive confirms carry their own mutation's failure. Without
          it the dialog simply closed on a refusal and the card looked exactly
          as it had before — a disconnect the server rejected was
          indistinguishable from one it accepted. */}
      <ConfirmModal
        open={disconnecting}
        confirmVariant="danger"
        title={t("provider.disconnectConfirm.title")}
        confirmLabel={t("provider.disconnect")}
        onConfirm={() => disconnect.mutate()}
        onClose={() => setDisconnecting(false)}
        pending={disconnect.isPending}
        error={
          disconnect.isError ? problemMessageOf(disconnect.error, t) : null
        }
      >
        {t("provider.disconnectConfirm.body")}
      </ConfirmModal>

      {/* Typed confirmation, like the data reset: this destroys purchased
          data on every contact, and a misclick must not be able to do it. */}
      <ConfirmModal
        open={deleting}
        confirmVariant="danger"
        title={t("provider.deleteDataConfirm.title")}
        confirmLabel={t("provider.deleteData")}
        confirmDisabled={
          typed.trim().toLowerCase() !== providerName(connection).toLowerCase()
        }
        onConfirm={() => deleteData.mutate()}
        pending={deleteData.isPending}
        error={
          deleteData.isError ? problemMessageOf(deleteData.error, t) : null
        }
        onClose={() => {
          setDeleting(false);
          setTyped("");
        }}
        initialFocusTo={() => phraseInput.current}
      >
        <ConfirmPhraseBox
          title={t("provider.deleteDataConfirm.impactTitle")}
          impact={[
            t("provider.deleteDataConfirm.impact.values"),
            t("provider.deleteDataConfirm.impact.spend"),
          ]}
          prompt={t("provider.deleteDataConfirm.typed")}
          phrase={providerName(connection)}
          value={typed}
          onChange={setTyped}
          inputRef={phraseInput}
        />
      </ConfirmModal>
    </>
  );
}
