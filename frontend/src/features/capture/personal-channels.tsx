// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MessageSquareShare, Sparkles } from "lucide-react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { ProviderList, ProviderRow } from "../../design-system/providerrow";
import { QueryStates } from "../../design-system/querystates";
import { formatDate, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";
import { usePassports } from "../../lib/passports.queries";
import { unwrap } from "../../lib/problem";

type Connection = components["schemas"]["PersonalChannelConnection"];

const CONNECTION_KEY = ["personal-channels", "connection"] as const;

/** Where a member reads how to connect an AI assistant. */
const AI_ASSISTANTS_HREF = "#/settings/agents";

function useConnection() {
  return useQuery({
    queryKey: CONNECTION_KEY,
    queryFn: async () =>
      unwrap(await api.GET("/personal-channels/connection")),
  });
}

/**
 * Personal channels: a member brings their own chats in through an AI
 * assistant and picks the replies up. Their own client is set up in the
 * developer section. Off until the member turns it on.
 */
export function PersonalChannelsCard() {
  const t = useT();
  const connection = useConnection();
  const queryClient = useQueryClient();
  const toggle = useMutation({
    mutationFn: async (enabled: boolean) =>
      unwrap(
        await api.PATCH("/personal-channels/connection", {
          body: { enabled },
        }),
      ),
    onSuccess: (next) => queryClient.setQueryData(CONNECTION_KEY, next),
  });
  const on = connection.data?.enabled ?? false;
  return (
    <Panel
      title={t("personalChannels.title")}
      titleAction={
        connection.isSuccess && (
          <Button
            variant={on ? "ghost" : "primary"}
            pending={toggle.isPending}
            onClick={() => toggle.mutate(!on)}
          >
            {on ? t("personalChannels.turnOff") : t("personalChannels.turnOn")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("personalChannels.intro")}</PanelIntro>
        <QueryStates
          query={connection}
          pendingLabel={t("personalChannels.loading")}
        >
          {connection.data && <Rows connection={connection.data} />}
        </QueryStates>
        <p className="t-caption">
          {t("personalChannels.ownClient")}{" "}
          <AppLink href={AI_ASSISTANTS_HREF}>
            {t("personalChannels.developers")}
          </AppLink>
        </p>
      </PanelBody>
    </Panel>
  );
}

function Rows({ connection }: Readonly<{ connection: Connection }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const plural = usePlural();
  const passports = usePassports();
  const assistant = (passports.data?.data ?? []).some(
    (p) =>
      p.connection !== undefined &&
      p.revoked_at == null &&
      p.scopes.includes("personal_channels"),
  );
  return (
    <ProviderList>
      <ProviderRow
        mark={<Sparkles aria-hidden="true" />}
        name={t("personalChannels.assistant")}
        status={
          assistant
            ? { tone: "success", label: t("personalChannels.connected") }
            : { tone: "default", label: t("personalChannels.notConnected") }
        }
        facts={t("personalChannels.assistantFacts")}
        action={
          <AppLink href={AI_ASSISTANTS_HREF}>
            {t("personalChannels.setUp")}
          </AppLink>
        }
      >
        <ol className="t-caption">
          <li>{t("personalChannels.step1")}</li>
          <li>{t("personalChannels.step2")}</li>
          <li>{t("personalChannels.step3")}</li>
        </ol>
      </ProviderRow>
      {connection.enabled && (
        <ProviderRow
          mark={<MessageSquareShare aria-hidden="true" />}
          name={t("personalChannels.replies")}
          status={{
            tone: connection.replies_waiting > 0 ? "warning" : "default",
            label: plural("personalChannels.waiting", connection.replies_waiting, {
              count: formatNumber(connection.replies_waiting, locale),
            }),
          }}
          facts={[
            connection.oldest_reply_waiting_at
              ? t("personalChannels.oldest", {
                  date: formatDate(connection.oldest_reply_waiting_at, locale, zone),
                })
              : null,
            connection.last_pull_at
              ? t("personalChannels.lastSeen", {
                  date: formatDate(connection.last_pull_at, locale, zone),
                })
              : t("personalChannels.neverSeen"),
          ]
            .filter(Boolean)
            .join(" · ")}
        />
      )}
    </ProviderList>
  );
}
