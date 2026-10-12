import { useCan, useCanWrite } from "../app/capability";
import { useInstallationSettings } from "../app/uploadlimit";
import { Callout } from "../design-system/callout";
import { NumberSettingRow } from "../design-system/numbersetting";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList } from "../design-system/settingrow";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate } from "./common";
import { useUpdateInstallationSettings } from "./installation-settings";

// The bound the API refuses past, mirrored so a value out of range is refused
// in the box before the request. Keyed by the wire property, which is how
// backend/gates/settingbounds_test.go holds it to the contract.
const TOKEN_BOUNDS = {
  oauth_access_token_ttl_minutes: { min: 5, max: 129_600 },
} as const;

// How long a connected agent's access token lives. It sits with sign-in
// because it is the same question — how long a credential this installation
// issued stays good — asked of an agent instead of a human.
//
// The page opens on `authentication_policy:read`, which a custom role can hold
// without `installation_settings:read`. A card narrower than its page withholds
// itself rather than drawing a refusal the reader can do nothing about.
export function AgentConnectionsCard() {
  const t = useT();
  const canRead = useCan("installation_settings", "read");
  const canManage = useCanWrite("installation_settings", "update");
  const query = useInstallationSettings(canRead);
  const update = useUpdateInstallationSettings(() => undefined);
  if (!canRead) {
    return null;
  }
  return (
    <Panel title={t("agentConnections.title")}>
      <PanelBody>
        <PanelIntro>{t("agentConnections.sub")}</PanelIntro>
        {!canManage && (
          <PanelIntro>{t("agentConnections.adminOnly")}</PanelIntro>
        )}
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("agentConnections.title")}>
        {(settings) => (
          <SettingList bleed="settings">
            <NumberSettingRow
              label={t("agentConnections.ttl.label")}
              description={t("agentConnections.ttl.help")}
              testId="agent-token-ttl"
              value={settings.oauth_access_token_ttl_minutes}
              {...TOKEN_BOUNDS.oauth_access_token_ttl_minutes}
              refusal={t("agentConnections.ttl.refusal")}
              disabled={!canManage || update.isPending}
              onCommit={(next) =>
                update.mutate({ oauth_access_token_ttl_minutes: next })
              }
            />
          </SettingList>
        )}
      </QueryGate>
      {update.isError && (
        <PanelBody>
          <Callout
            tone="danger"
            kind="outcome"
            title={t("agentConnections.updateFailed")}
          >
            {problemMessageOf(update.error, t)}
          </Callout>
        </PanelBody>
      )}
    </Panel>
  );
}
