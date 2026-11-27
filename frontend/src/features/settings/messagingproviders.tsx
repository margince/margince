// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { MessengerAccountActions } from "../../components/messenger/messenger.account";
import {
  MESSENGER_PROVIDERS,
  type MessengerProvider,
  useMessengerIdentity,
  useMessengerSetup,
} from "../../components/messenger/messenger.queries";
import { Disclosure } from "../../design-system/atoms";
import { CopyAction } from "../../design-system/clipboardcopy";
import { Panel, PanelBody } from "../../design-system/panel";
import { ProviderMark } from "../../design-system/provider-mark";
import { ProviderList, ProviderRow } from "../../design-system/providerrow";
import { QueryStates } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useViewerId } from "../../lib/session";
import { ChannelInstallations } from "./channelinstallations";
import { OAuthProviderRow } from "./oauth-apps-card";

export function MessagingProvidersPanel() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  const reason = useId();
  return (
    <>
      <Panel title={t("messenger.myAccounts")}>
        <PanelBody>
          <ProviderList>
            {MESSENGER_PROVIDERS.map((provider) => (
              <ParticipantRow
                key={provider.value}
                provider={provider.value}
                name={provider.label}
              />
            ))}
          </ProviderList>
        </PanelBody>
      </Panel>
      <ChannelInstallations />
      <Panel title={t("messenger.apps")}>
        <PanelBody>
          {!canManage && <p id={reason}>{t("oauthApp.noWriteReason")}</p>}
          <ProviderList>
            <OAuthProviderRow
              provider="slack"
              refusedBy={canManage ? undefined : reason}
            />
            <OAuthProviderRow
              provider="microsoft"
              refusedBy={canManage ? undefined : reason}
            />
          </ProviderList>
        </PanelBody>
      </Panel>
    </>
  );
}

function ParticipantRow({
  provider,
  name,
}: Readonly<{ provider: MessengerProvider; name: string }>) {
  const t = useT();
  const seat = useViewerId();
  const identity = useMessengerIdentity(provider, Boolean(seat), seat);
  const setup = useMessengerSetup(provider);
  const value = identity.data;
  return (
    <ProviderRow
      mark={<ProviderMark providerKey={provider} />}
      name={name}
      account={value?.sender?.name}
    >
      <QueryStates query={identity} pendingLabel={t("messenger.loading")}>
        <MessengerAccountActions
          provider={provider}
          state={value?.state}
          showSettings={false}
        />
      </QueryStates>
      <Disclosure summary={t("messenger.setupDetails")}>
        <QueryStates query={setup} pendingLabel={t("messenger.loading")}>
          {setup.data && (
            <>
              <p>
                {t(
                  setup.data.configured
                    ? "messenger.appReady"
                    : "messenger.appMissing",
                )}
              </p>
              {provider === "teams" && <p>{t("messenger.teamsTenant")}</p>}
              {setup.data.callback_url && (
                <CopyAction
                  text={setup.data.callback_url}
                  labels={{
                    copy: t("oauthApp.copy"),
                    copied: t("oauthApp.redirectCopied"),
                    remedy: t("oauthApp.redirectCopyFailed"),
                  }}
                />
              )}
              <p>{t("messenger.participantScopes")}</p>
              <ul>
                {setup.data.participant_scopes.map((scope) => (
                  <li key={scope}>{scope}</li>
                ))}
              </ul>
            </>
          )}
        </QueryStates>
      </Disclosure>
    </ProviderRow>
  );
}
