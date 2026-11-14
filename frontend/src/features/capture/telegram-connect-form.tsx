// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Badge,
  Button,
  Field,
  Modal,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Heading } from "../../design-system/heading";
import { useT } from "../../i18n";
import { statusLabel, statusTone } from "../../lib/connector-status";
import { problemMessageOf, unwrap } from "../../lib/problem";
import "../../design-system/querystates.css";
import { BOT_COPY, type BotProvider } from "./botcopy";

// The Telegram connector: one bot connects for the WHOLE workspace, not
// per-user — there is no OAuth handshake, so first-connect is the same "paste a
// credential, submit" shape imap-connect-form.tsx already established, and it
// asks for nothing else: the installation polls the provider for messages, so
// there is no address of our own for anyone to supply. Unlike the mail
// providers, a live channel connection stays EDITABLE: replacing the token goes
// through PATCH and stays `connected` throughout, so captured history and every
// channel identity binding survive the rotation instead of a
// disconnect-reconnect cycle that would discard them. This one form serves both
// first-connect (no `connection` prop) and that in-place edit (a `connection`
// prop supplies the id PATCH targets). A self-hosted provider (Dispact) adds
// its chat's address on first connect; a token rotation keeps the address.
// Dispact also takes an optional events secret, on connect and on rotation;
// left empty on a rotation, the saved one stays.

type ChannelConnection = components["schemas"]["ChannelConnection"];

type BotCredentials = Readonly<{
  token: string;
  address: string;
  eventsSecret: string;
}>;

export function BotConnectForm({
  provider = "telegram",
  open,
  onClose,
  connection,
  onConnected,
}: Readonly<{
  provider?: BotProvider;
  open: boolean;
  onClose: () => void;
  // Present only when replacing an existing connection's token; absent for
  // the first connect. Its presence alone decides PATCH vs POST below —
  // there is no separate "mode" flag to drift out of sync with it.
  connection?: ChannelConnection;
  onConnected?: () => void;
}>) {
  const t = useT();
  const headingId = useId();
  const formId = useId();
  const queryClient = useQueryClient();
  const [botToken, setBotToken] = useState("");
  const [address, setAddress] = useState("");
  const [eventsSecret, setEventsSecret] = useState("");
  const copy = BOT_COPY[provider];
  // Only a new Dispact binding names its chat; the server refuses an address
  // for Telegram, and a token rotation takes the token alone.
  const asksAddress = provider === "dispact" && !connection;
  // Only Dispact pushes signed notices; the server refuses a secret for Telegram.
  const asksEventsSecret = provider === "dispact";

  const connect = useMutation({
    mutationFn: async ({
      token,
      address: chatAddress,
      eventsSecret: secret,
    }: BotCredentials): Promise<ChannelConnection> => {
      const withSecret = secret ? { eventsSecret: secret } : {};
      if (connection) {
        return unwrap(
          await api.PATCH("/channel-connections/{id}", {
            params: { path: { id: connection.id } },
            body: { botToken: token, ...withSecret },
          }),
          t,
        );
      }
      return unwrap(
        await api.POST("/channel-connections", {
          body: {
            provider,
            botToken: token,
            ...(chatAddress ? { address: chatAddress } : {}),
            ...withSecret,
          },
        }),
        t,
      );
    },
    onSuccess: () => {
      // Never claim a connection the server did not confirm: the list is
      // the proof, so invalidate it and let the panel's own re-read of
      // GET /channel-connections drive whatever it shows next.
      queryClient.invalidateQueries({ queryKey: ["channel-connections"] });
      setBotToken("");
      setEventsSecret("");
    },
    onError: () => {
      // The token is never retained after a failed submit — the same
      // posture the IMAP form holds for its secret.
      setBotToken("");
      setEventsSecret("");
    },
  });

  // A fresh open starts from nothing: neither a previous attempt's token nor
  // the outcome it reached. This form is mounted for the life of the Settings
  // card and only shown or hidden, so a retained success view is what the
  // admin meets on the next rotation — a bot username and a Done button,
  // with no way back to the token field.
  const { reset: resetConnect } = connect;
  const wasOpen = useRef(false);
  useEffect(() => {
    if (open && !wasOpen.current) {
      setBotToken("");
      setAddress("");
      setEventsSecret("");
      resetConnect();
    }
    wasOpen.current = open;
  }, [open, resetConnect]);

  const ready =
    botToken.trim() !== "" && (!asksAddress || address.trim() !== "");
  // RFC 7807 `detail` carries the actionable reason (e.g. which of the two
  // conflicts a 409 is — this bot is bound elsewhere, or this workspace already
  // has one) — surfaced verbatim rather than flattened into a generic "failed
  // to connect".
  const errorMessage = connect.isError
    ? problemMessageOf(connect.error, t)
    : null;
  const resolved = connect.isSuccess ? connect.data : null;

  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {connection ? t("connectors.telegramEditTitle") : t(copy.modalTitle)}
      </Heading>
      {resolved ? (
        <>
          <div className="form-stack">
            <p className="t-body">
              {t("connectors.telegramConnectedAs", {
                username: resolved.channelLabel,
              })}
            </p>
            <div>
              <Badge tone={statusTone(resolved.status)}>
                {t(statusLabel(resolved.status))}
              </Badge>
            </div>
          </div>
          <div className="actions">
            <Button
              variant="primary"
              onClick={() => {
                onConnected?.();
                onClose();
              }}
            >
              {t("webhooks.secret.done")}
            </Button>
          </div>
        </>
      ) : (
        <>
          <form
            id={formId}
            className="form-stack"
            onSubmit={(event) => {
              event.preventDefault();
              if (!ready || connect.isPending) {
                return;
              }
              connect.mutate({
                token: botToken.trim(),
                address: asksAddress ? address.trim() : "",
                eventsSecret: asksEventsSecret ? eventsSecret.trim() : "",
              });
            }}
          >
            {/* A parked binding still reads as parked while its token is replaced. */}
            {connection && (
              <div>
                <Badge tone={statusTone(connection.status)}>
                  {t(statusLabel(connection.status))}
                </Badge>
              </div>
            )}
            {asksAddress && (
              <Field
                label={t("connectors.bot.dispactAddress")}
                required
                hint={t("connectors.bot.dispactAddressHint")}
              >
                {(control) => (
                  <TextInput
                    {...control}
                    type="url"
                    autoComplete="off"
                    value={address}
                    onChange={(event) => setAddress(event.target.value)}
                  />
                )}
              </Field>
            )}
            <Field
              label={t("connectors.telegramBotToken")}
              required
              hint={t(copy.tokenHint)}
            >
              {(control) => (
                <TextInput
                  {...control}
                  type="password"
                  autoComplete="off"
                  value={botToken}
                  onChange={(event) => setBotToken(event.target.value)}
                />
              )}
            </Field>
            {asksEventsSecret && (
              <EventsSecretField
                value={eventsSecret}
                onChange={setEventsSecret}
                keepsSaved={connection?.eventsSecretSet === true}
              />
            )}
            {errorMessage && (
              <Callout
                tone="danger"
                kind="outcome"
                title={t("connectors.telegramConnectFailed")}
              >
                {errorMessage}
              </Callout>
            )}
          </form>
          <div className="actions">
            <Button
              type="button"
              onClick={onClose}
              disabled={connect.isPending}
            >
              {t("create.cancel")}
            </Button>
            <Button
              variant="primary"
              type="submit"
              form={formId}
              disabled={!connect.isPending && !ready}
              pending={connect.isPending}
              busyLabel={t("create.saving")}
            >
              {connection
                ? t("connectors.telegramReplaceCta")
                : t("connectors.telegramSubmitCta")}
            </Button>
          </div>
        </>
      )}
    </Modal>
  );
}

// The optional Dispact events secret. On a rotation an empty field keeps the
// saved secret, so the hint says so once one is saved.
function EventsSecretField({
  value,
  onChange,
  keepsSaved,
}: Readonly<{
  value: string;
  onChange: (value: string) => void;
  keepsSaved: boolean;
}>) {
  const t = useT();
  return (
    <Field
      label={t("connectors.bot.eventsSecret")}
      hint={t(
        keepsSaved
          ? "connectors.bot.eventsSecretKeepHint"
          : "connectors.bot.eventsSecretHint",
      )}
    >
      {(control) => (
        <TextInput
          {...control}
          type="password"
          autoComplete="off"
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );
}
