// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import { useConnectors } from "../../lib/connectors";

/**
 * The address this installation puts in the links it emails, and whether it
 * answered when last asked.
 *
 * On System health because it answers that page's question: is something
 * broken in the background. A localhost origin sends mail whose unsubscribe
 * links open nothing. Reported, never enforced: a probe from inside the
 * deployment cannot show a recipient's client can reach the address.
 */
export function PublicOriginCard() {
  const t = useT();
  // GET /connectors carries the origin, so this shares the roster's cache entry.
  const status = useConnectors().data?.publicOrigin;
  // No origin configured is not a broken origin: there is nothing to report.
  if (!status) {
    return null;
  }
  const checked = status.reachable !== null && status.reachable !== undefined;
  const tone = checked ? (status.reachable ? "success" : "warning") : undefined;
  const stateLabel = checked
    ? t(
        status.reachable
          ? "connectors.originReachable"
          : "connectors.originUnreachable",
      )
    : t("connectors.originUnchecked");
  return (
    <Panel title={t("connectors.originTitle")}>
      <PanelBody>
        <PanelIntro>{t("connectors.originSub")}</PanelIntro>
        <SettingList>
          <SettingRow
            testId="public-origin"
            label={t("connectors.originLabel")}
            description={status.origin}
            control={<Badge tone={tone}>{stateLabel}</Badge>}
          />
        </SettingList>
      </PanelBody>
    </Panel>
  );
}
