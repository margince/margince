// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ChannelHistoryProfile } from "../../components/channelhistoryprofile";
import { Button, Disclosure } from "../../design-system/atoms";
import { CopyAction } from "../../design-system/clipboardcopy";
import { Field, TextInput } from "../../design-system/field";
import { Panel, PanelBody } from "../../design-system/panel";
import { ProviderMark } from "../../design-system/provider-mark";
import { ProviderList, ProviderRow } from "../../design-system/providerrow";
import { QueryStates, WriteRefused } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { downloadBytes } from "../../lib/download";
import { unwrap } from "../../lib/problem";

export function ChannelInstallations() {
  const t = useT();
  return (
    <Panel title={t("channelInstall.title")}>
      <PanelBody>
        <p>{t("channelInstall.separate")}</p>
        <ProviderList>
          <SlackInstallation />
          <TeamsInstallation />
        </ProviderList>
      </PanelBody>
    </Panel>
  );
}

function SetupAddress({ address }: Readonly<{ address: string }>) {
  const t = useT();
  return (
    <CopyAction
      text={address}
      labels={{
        copy: t("oauthApp.copy"),
        copied: t("oauthApp.redirectCopied"),
        remedy: t("oauthApp.redirectCopyFailed"),
      }}
    />
  );
}

function SlackInstallation() {
  const t = useT();
  const [distribution, setDistribution] =
    useState<components["schemas"]["SlackHistoryProfile"]["distribution"]>(
      "internal",
    );
  const canRead = useCan("channel_connection", "read");
  const canInstall = useCanWrite("channel_connection", "create");
  const setup = useQuery({
    queryKey: ["channel-installation", "slack"],
    enabled: canRead,
    queryFn: async () =>
      unwrap(await api.GET("/channel-providers/slack/setup")),
  });
  const connect = useMutation({
    mutationFn: async (
      distribution: components["schemas"]["SlackHistoryProfile"]["distribution"],
    ) =>
      unwrap(
        await api.POST("/channel-providers/slack/connect", {
          body: { distribution },
        }),
      ),
    onSuccess: (result) => globalThis.location.assign(result.authorize_url),
  });
  return (
    <ProviderRow mark={<ProviderMark providerKey="slack" />} name="Slack">
      <p>{t("channelInstall.slackHistoryProfile")}</p>
      <p>{t("channelInstall.slackReader")}</p>
      {canRead && (
        <QueryStates query={setup} pendingLabel={t("messenger.loading")}>
          <p>
            {t(
              setup.data?.configured
                ? "channelInstall.slackReady"
                : "channelInstall.slackMissing",
            )}
          </p>
          <ChannelHistoryProfile profile={setup.data?.history_profile} />
          <Field
            label={t("channelProfile.distribution")}
            hint={t("channelProfile.declaration")}
          >
            {(control) => (
              <Select
                {...control}
                value={distribution}
                disabled={!canInstall || connect.isPending}
                options={[
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
                onChange={(value) => {
                  if (
                    value === "internal" ||
                    value === "marketplace" ||
                    value === "non_marketplace"
                  )
                    setDistribution(value);
                }}
              />
            )}
          </Field>
          {distribution === "non_marketplace" && (
            <p>{t("channelProfile.unsupported")}</p>
          )}
          <Button
            disabled={!canInstall || !setup.data?.configured}
            pending={connect.isPending}
            onClick={() => connect.mutate(distribution)}
          >
            {t("channelInstall.slackConnect")}
          </Button>
          <WriteRefused
            titleKey="channelInstall.failed"
            error={connect.error}
          />
          {setup.data && (
            <Disclosure summary={t("messenger.setupDetails")}>
              <p>{t("channelInstall.callback")}</p>
              <SetupAddress address={setup.data.callback_url} />
              <p>{t("channelInstall.events")}</p>
              <SetupAddress address={setup.data.events_url} />
              <ul>
                {setup.data.scopes.map((scope) => (
                  <li key={scope}>{scope}</li>
                ))}
              </ul>
            </Disclosure>
          )}
        </QueryStates>
      )}
      {!canInstall && <p>{t("channelInstall.adminRequired")}</p>}
    </ProviderRow>
  );
}

function TeamsInstallation() {
  const t = useT();
  const client = useQueryClient();
  const canRead = useCan("channel_connection", "read");
  const canInstall = useCanWrite("channel_connection", "create");
  const [installation, choose] = useState("");
  const setup = useQuery({
    queryKey: ["channel-installation", "teams"],
    enabled: canRead,
    queryFn: async () =>
      unwrap(await api.GET("/channel-providers/teams/setup")),
  });
  const connect = useMutation({
    mutationFn: async (installationId: string) =>
      unwrap(
        await api.POST("/channel-providers/teams/connect", {
          body: { installation_id: installationId },
        }),
      ),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["channel-connections"] });
    },
  });
  return (
    <ProviderRow
      mark={<ProviderMark providerKey="teams" />}
      name="Microsoft Teams"
    >
      {canRead && (
        <QueryStates query={setup} pendingLabel={t("messenger.loading")}>
          <p>{t("channelInstall.teamsSteps")}</p>
          {setup.data?.configured ? (
            <>
              <p>
                {t("channelInstall.teamsApp", {
                  app: setup.data.app_id,
                  tenant: setup.data.tenant_id,
                })}
              </p>
              <Disclosure summary={t("messenger.setupDetails")}>
                <p>{t("channelInstall.events")}</p>
                <SetupAddress address={setup.data.events_url} />
                <ul>
                  {setup.data.scopes.map((scope) => (
                    <li key={scope}>{scope}</li>
                  ))}
                </ul>
              </Disclosure>
              <TeamsPackage disabled={!canInstall} />
              <Button
                pending={setup.isFetching}
                onClick={() => setup.refetch()}
              >
                {t("channelInstall.refresh")}
              </Button>
              {setup.data.installations.length === 0 ? (
                <p>{t("channelInstall.noTeams")}</p>
              ) : (
                <>
                  <Field label={t("channelInstall.team")}>
                    {(control) => (
                      <Select
                        {...control}
                        value={installation}
                        disabled={connect.isPending}
                        onChange={choose}
                        options={setup.data.installations.map((item) => ({
                          value: item.id,
                          label: `${item.name} (${item.team_id})`,
                        }))}
                      />
                    )}
                  </Field>
                  <p>{t("channelInstall.approval")}</p>
                  <Button
                    disabled={
                      !canInstall ||
                      !setup.data.installations.some(
                        (item) => item.id === installation,
                      )
                    }
                    pending={connect.isPending}
                    onClick={() => connect.mutate(installation)}
                  >
                    {t("channelInstall.approve")}
                  </Button>
                </>
              )}
              {connect.isSuccess && connect.variables === installation && (
                <p role="status">{t("channelInstall.connected")}</p>
              )}
              <WriteRefused
                titleKey="channelInstall.failed"
                error={connect.error}
              />
            </>
          ) : (
            <p>{t("channelInstall.teamsMissing")}</p>
          )}
        </QueryStates>
      )}
      {!canInstall && <p>{t("channelInstall.adminRequired")}</p>}
    </ProviderRow>
  );
}

function TeamsPackage({ disabled }: Readonly<{ disabled: boolean }>) {
  const t = useT();
  const [privacy, setPrivacy] = useState("");
  const [terms, setTerms] = useState("");
  const download = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/channel-providers/teams/package", {
          body: { privacy_url: privacy, terms_url: terms },
          parseAs: "blob",
        }),
      ),
    onSuccess: (bytes) =>
      downloadBytes(bytes, "margince-teams.zip", "application/zip"),
  });
  return (
    <Disclosure summary={t("channelInstall.package")}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          download.mutate();
        }}
      >
        <p>{t("channelInstall.legal")}</p>
        <Field label={t("channelInstall.privacy")} required>
          {(control) => (
            <TextInput
              {...control}
              type="url"
              required
              maxLength={8192}
              value={privacy}
              onChange={(event) => setPrivacy(event.target.value)}
              disabled={disabled || download.isPending}
            />
          )}
        </Field>
        <Field label={t("channelInstall.terms")} required>
          {(control) => (
            <TextInput
              {...control}
              type="url"
              required
              maxLength={8192}
              value={terms}
              onChange={(event) => setTerms(event.target.value)}
              disabled={disabled || download.isPending}
            />
          )}
        </Field>
        <Button
          type="submit"
          disabled={
            disabled ||
            !privacy.startsWith("https://") ||
            !terms.startsWith("https://")
          }
          pending={download.isPending}
        >
          {t("channelInstall.download")}
        </Button>
        <WriteRefused titleKey="channelInstall.failed" error={download.error} />
      </form>
    </Disclosure>
  );
}
