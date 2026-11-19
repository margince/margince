// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge, Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatDateTime, formatNumber } from "../../format/format";
import { formatRelativeTime } from "../../format/relativetime";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { deviceLabel } from "./devicelabel";
import {
  type MySession,
  useEndSessions,
  useMySessions,
} from "./signeddevices.queries";

function nameOf(session: MySession, t: Translator): string {
  const { browser, system } = deviceLabel(session.user_agent);
  if (browser && system)
    return t("account.devicesBrowserOn", { browser, system });
  return browser ?? system ?? t("account.devicesUnknown");
}

// Other sessions can be ended from here. This one is marked and offers no
// button, because the Sign out row above it ends it.
export function SignedInDevicesPanel() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const sessions = useMySessions();
  const end = useEndSessions();
  return (
    <Panel title={t("account.devicesCard")}>
      <QueryGate query={sessions} pendingLabel={t("account.devicesCard")}>
        {(all) => {
          const others = all.filter((session) => !session.current);
          return (
            <SettingList bleed="records">
              {all.map((session) => (
                <SessionRow
                  key={session.id}
                  session={session}
                  pending={end.isPending}
                  onEnd={() => end.mutate([session.id])}
                />
              ))}
              {others.length > 0 && (
                <SettingRow
                  label={t("account.devicesOthers")}
                  description={t("account.devicesOthersHelp")}
                  control={
                    <Button
                      variant="ghost"
                      disabled={end.isPending}
                      onClick={() => end.mutate(others.map((s) => s.id))}
                    >
                      {t("account.devicesSignOutOthers")}
                    </Button>
                  }
                />
              )}
            </SettingList>
          );
        }}
      </QueryGate>
      {(end.isError || (end.data ?? 0) > 0) && (
        <PanelBody>
          <ErrorLine>
            {end.isError
              ? problemMessageOf(end.error, t)
              : plural("account.devicesFailed", end.data ?? 0, {
                  count: formatNumber(end.data ?? 0, locale),
                })}
          </ErrorLine>
        </PanelBody>
      )}
    </Panel>
  );
}

function SessionRow({
  session,
  pending,
  onEnd,
}: Readonly<{ session: MySession; pending: boolean; onEnd: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const name = nameOf(session, t);
  return (
    <SettingRow
      label={name}
      description={t("account.devicesWhen", {
        signedIn: formatDateTime(session.signed_in_at, locale, zone),
        lastActive: formatRelativeTime(session.last_active_at, locale),
      })}
      control={
        session.current ? (
          <Badge>{t("account.devicesThisOne")}</Badge>
        ) : (
          <Button
            variant="ghost"
            aria-label={t("account.devicesSignOutNamed", { device: name })}
            disabled={pending}
            onClick={onEnd}
          >
            {t("account.devicesSignOut")}
          </Button>
        )
      }
    />
  );
}
