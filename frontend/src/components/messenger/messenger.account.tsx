// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useT } from "../../i18n";
import { useCanMutate, useCanWrite } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { DispactParticipantConnect } from "./messenger.dispact";
import {
  type MessengerProvider,
  type MessengerScope,
  useMessengerConnect,
} from "./messenger.queries";

function useDisconnectMessenger() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({
      provider,
    }: Readonly<{ provider: MessengerProvider }>) =>
      unwrap(
        await api.DELETE("/messenger/{provider}/identity", {
          params: { path: { provider } },
        }),
      ),
    onSuccess: async () => {
      for (const key of [
        "messenger-context",
        "messenger-messages",
        "messenger-channels",
        "messenger-members",
        "messenger-team",
      ]) {
        await client.cancelQueries({ queryKey: [key] });
        client.removeQueries({ queryKey: [key] });
      }
      await client.invalidateQueries({ queryKey: ["messenger-identity"] });
    },
  });
}

export function MessengerAccountActions({
  state,
  provider,
  showSettings = true,
  compact = false,
  returnRecord,
}: Readonly<{
  state: components["schemas"]["MessengerIdentity"]["state"] | undefined;
  provider: MessengerProvider;
  showSettings?: boolean;
  compact?: boolean;
  returnRecord?: Pick<MessengerScope, "record_type" | "id">;
}>) {
  if (state === "connected")
    return <DisconnectMessengerAccount provider={provider} compact={compact} />;
  if (state === undefined || compact) return null;
  if (provider === "dispact")
    return (
      <div className="messenger-compose">
        <DispactParticipantConnect />
        {showSettings && <MessengerSettingsAction />}
      </div>
    );
  return (
    <ConnectMessengerAccount
      provider={provider}
      state={state}
      showSettings={showSettings}
      returnRecord={returnRecord}
    />
  );
}

function DisconnectMessengerAccount({
  provider,
  compact,
}: Readonly<{ provider: MessengerProvider; compact: boolean }>) {
  const t = useT();
  const canMutate = useCanMutate();
  const disconnect = useDisconnectMessenger();
  const [confirm, Confirm] = useState(false);
  return (
    <>
      {compact ? (
        <OverflowMenu label={t("messenger.accountMenu")}>
          <Button disabled={!canMutate} onClick={() => Confirm(true)}>
            {t("messenger.disconnect")}
          </Button>
        </OverflowMenu>
      ) : (
        <Button disabled={!canMutate} onClick={() => Confirm(true)}>
          {t("messenger.disconnect")}
        </Button>
      )}
      <ConfirmModal
        open={confirm}
        onClose={() => Confirm(false)}
        title={t("messenger.disconnectTitle")}
        confirmLabel={t("messenger.disconnect")}
        pending={disconnect.isPending}
        error={disconnect.error ? problemMessageOf(disconnect.error, t) : null}
        onConfirm={() =>
          disconnect.mutate({ provider }, { onSuccess: () => Confirm(false) })
        }
      >
        <p>{t("messenger.disconnectBody")}</p>
      </ConfirmModal>
    </>
  );
}

function ConnectMessengerAccount({
  provider,
  state,
  showSettings,
  returnRecord,
}: Readonly<{
  provider: MessengerProvider;
  state: "needs_consent" | "unavailable";
  showSettings: boolean;
  returnRecord?: Pick<MessengerScope, "record_type" | "id">;
}>) {
  const t = useT();
  const canMutate = useCanMutate();
  const connect = useMessengerConnect();
  return (
    <div className="messenger-compose">
      <p>
        {t(
          state === "needs_consent"
            ? "messenger.needsConsent"
            : "messenger.unavailable",
        )}
      </p>
      {!canMutate && <p>{t("settings.seatRefused")}</p>}
      {state === "needs_consent" && (
        <Button
          variant="primary"
          pending={connect.isPending}
          disabled={!canMutate}
          onClick={() =>
            connect.mutate(
              { provider, returnRecord },
              {
                onSuccess: (result) =>
                  globalThis.location.assign(result.authorize_url),
              },
            )
          }
        >
          {t("messenger.connect")}
        </Button>
      )}
      {showSettings && <MessengerSettingsAction />}
      {connect.error && (
        <Callout tone="warning" title={t("messenger.connectProblem")}>
          {problemMessageOf(connect.error, t)}
        </Callout>
      )}
    </div>
  );
}

function MessengerSettingsAction() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  if (!canManage) return null;
  return (
    <Button onClick={() => navigate(settingsHref("integrations", "telegram"))}>
      {t("messenger.settings")}
    </Button>
  );
}
