// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  MESSENGER_PROVIDERS,
  type MessengerProvider,
} from "../../components/messenger/messenger.queries";
import { useMessengerService } from "../../components/messenger/messenger.service";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { ProviderMark } from "../../design-system/provider-mark";
import { QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";

type Distribution =
  components["schemas"]["SlackHistoryProfile"]["distribution"];
type Selection = Readonly<{
  provider: MessengerProvider;
  distribution?: Distribution;
  disconnect: boolean;
}>;

export function MessagingServiceSwitch() {
  const t = useT();
  const service = useMessengerService();
  const canSwitch = useCanWrite("channel_connection", "delete");
  const client = useQueryClient();
  const [intent, Intent] = useState<"select" | "disconnect" | null>(null);
  const [choice, Choose] = useState<MessengerProvider | null>(null);
  const [plan, Plan] = useState<Distribution | null>(null);
  const active = service.data?.active_provider ?? service.data?.providers?.[0];
  const selected = choice ?? active ?? "dispact";
  const distribution = plan ?? service.data?.history_profile?.distribution;
  const change = useMutation({
    mutationFn: async (input: Selection) =>
      input.disconnect
        ? unwrap(
            await api.DELETE("/messenger/service/{provider}", {
              params: { path: { provider: input.provider } },
            }),
          )
        : unwrap(
            await api.PUT("/messenger/service", {
              body: {
                provider: input.provider,
                slack_distribution: input.distribution,
              },
            }),
          ),
    onSuccess: () => Intent(null),
    onSettled: async () => {
      await client.cancelQueries({
        predicate: (query) =>
          typeof query.queryKey[0] === "string" &&
          query.queryKey[0].startsWith("messenger-"),
      });
      client.removeQueries({
        predicate: (query) =>
          typeof query.queryKey[0] === "string" &&
          query.queryKey[0].startsWith("messenger-"),
      });
      await client.invalidateQueries();
    },
  });
  const unchanged =
    active === selected &&
    service.data?.providers?.length === 1 &&
    (selected !== "slack" ||
      distribution === service.data?.history_profile?.distribution);
  return (
    <Panel title={t("messenger.serviceTitle")}>
      <PanelBody>
        <p>{t("messenger.oneService")}</p>
        <QueryStates query={service} pendingLabel={t("messenger.loading")}>
          {!active && <p>{t("messenger.chooseService")}</p>}
          {(service.data?.providers?.length ?? 0) > 1 && (
            <ErrorLine standing>{t("messenger.serviceConflict")}</ErrorLine>
          )}
          {service.data?.providers?.map((provider) => (
            <div key={provider}>
              <ProviderMark providerKey={provider} />{" "}
              {
                MESSENGER_PROVIDERS.find((item) => item.value === provider)
                  ?.label
              }
            </div>
          ))}
        </QueryStates>
        <Select
          aria-label={t("messenger.serviceTitle")}
          value={selected}
          options={MESSENGER_PROVIDERS.map((item) => ({
            ...item,
            adornment: <ProviderMark providerKey={item.value} />,
          }))}
          disabled={!canSwitch}
          onChange={(value) => {
            const provider = MESSENGER_PROVIDERS.find(
              (item) => item.value === value,
            );
            if (provider) Choose(provider.value);
          }}
        />
        {selected === "slack" && (
          <SlackDistributionChoice
            value={distribution}
            enabled={canSwitch}
            onChange={Plan}
          />
        )}
        <ServiceChoiceAction
          canSwitch={canSwitch}
          ready={service.isSuccess}
          unchanged={unchanged}
          needsPlan={selected === "slack" && !distribution}
          hasService={Boolean(active)}
          onClick={() => Intent("select")}
        />
        {active && (
          <Button
            disabled={!canSwitch || change.isPending}
            onClick={() => Intent("disconnect")}
          >
            {t("messenger.disconnectService")}
          </Button>
        )}
        <ServiceConfirmation
          intent={intent}
          provider={selected}
          active={active}
          distribution={distribution}
          pending={change.isPending}
          error={change.error}
          onClose={() => Intent(null)}
          onConfirm={change.mutate}
        />
      </PanelBody>
    </Panel>
  );
}

function ServiceConfirmation({
  intent,
  provider,
  active,
  distribution,
  pending,
  error,
  onClose,
  onConfirm,
}: Readonly<{
  intent: "select" | "disconnect" | null;
  provider: MessengerProvider;
  active?: MessengerProvider;
  distribution?: Distribution;
  pending: boolean;
  error: Error | null;
  onClose: () => void;
  onConfirm: (selection: Selection) => void;
}>) {
  const t = useT();
  return (
    <ConfirmModal
      open={intent !== null}
      onClose={onClose}
      title={t(
        intent === "disconnect"
          ? "messenger.disconnectService"
          : "messenger.switchService",
      )}
      confirmLabel={t(
        intent === "disconnect"
          ? "messenger.disconnectService"
          : "messenger.useService",
      )}
      pending={pending}
      error={error ? problemMessageOf(error, t) : null}
      onConfirm={() => {
        if (intent === "disconnect" && active)
          onConfirm({ provider: active, disconnect: true });
        else
          onConfirm({
            provider,
            distribution: provider === "slack" ? distribution : undefined,
            disconnect: false,
          });
      }}
    >
      <p>
        {intent === "disconnect"
          ? t("messenger.disconnectServiceBody")
          : t("messenger.switchServiceBody", {
              name:
                MESSENGER_PROVIDERS.find((item) => item.value === provider)
                  ?.label ?? provider,
            })}
      </p>
    </ConfirmModal>
  );
}

function SlackDistributionChoice({
  value,
  enabled,
  onChange,
}: Readonly<{
  value?: Distribution;
  enabled: boolean;
  onChange: (value: Distribution) => void;
}>) {
  const t = useT();
  return (
    <>
      <p>{t("messenger.slackPlan")}</p>
      <Select
        aria-label={t("channelProfile.distribution")}
        value={value ?? ""}
        options={[
          {
            value: "",
            label: t("messenger.chooseSlackPlan"),
            disabled: true,
          },
          { value: "internal", label: t("channelProfile.internal") },
          {
            value: "marketplace",
            label: t("channelProfile.marketplace"),
          },
          {
            value: "non_marketplace",
            label: t("channelProfile.nonMarketplace"),
          },
        ]}
        disabled={!enabled}
        onChange={(value) => {
          if (
            value === "internal" ||
            value === "marketplace" ||
            value === "non_marketplace"
          )
            onChange(value);
        }}
      />
    </>
  );
}

function ServiceChoiceAction({
  canSwitch,
  ready,
  unchanged,
  needsPlan,
  hasService,
  onClick,
}: Readonly<{
  canSwitch: boolean;
  ready: boolean;
  unchanged: boolean;
  needsPlan: boolean;
  hasService: boolean;
  onClick: () => void;
}>) {
  const t = useT();
  return (
    <Button
      reason={
        !canSwitch
          ? t("channelInstall.adminRequired")
          : needsPlan
            ? t("messenger.chooseSlackPlan")
            : undefined
      }
      disabled={!canSwitch || !ready || unchanged || needsPlan}
      onClick={onClick}
    >
      {t(hasService ? "messenger.switchService" : "messenger.useService")}
    </Button>
  );
}
