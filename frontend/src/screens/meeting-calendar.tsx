// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarClock, Send } from "lucide-react";
import { useRef, useState } from "react";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { Button, OverflowMenu, PendingBody } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody } from "../design-system/panel";
import { ProviderMark } from "../design-system/provider-mark";
import { ProviderList, ProviderRow } from "../design-system/providerrow";
import { Chip } from "../design-system/readings";
import { MultiSelect, Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { formatElapsed, useNow } from "../format/now";
import { useLocale, usePlural, useT } from "../i18n";
import { calendarName, useMeetingCalendar } from "./booking-calendar-state";
import { QueryGate } from "./common";
import { notConfiguredProvider, useConnectAccount } from "./connect-account";
import { missingCalendarWriteGrant } from "./connector-status";
import { vendorOf } from "./connectorproviders";
import { usePatchSchedulingProfile } from "./scheduling-profile-query";
import "./meeting-cards.css";

type Profile = components["schemas"]["SchedulingProfile"];
type CaptureConnection = components["schemas"]["CaptureConnection"];

/** What a connect press left to say: the vendor is not wired here, or it failed. */
export function ConnectRefusal({
  connect,
}: Readonly<{ connect: ReturnType<typeof useConnectAccount> }>) {
  const t = useT();
  const unwired = notConfiguredProvider(connect.data);
  return (
    <>
      {unwired && (
        <p className="t-caption">
          {t("connectors.providerNotConfigured", {
            provider: calendarName(t, unwired) || unwired,
          })}
        </p>
      )}
      <ErrorLine error={connect.error} />
    </>
  );
}

/**
 * The calendar each booking lands in and the ones that block time, saved from
 * the card foot: blocking is a multi-pick, and every save runs the live
 * calendar check, so a write per tick would check the calendar per tick.
 */
export function MeetingCalendarCard({
  profile,
}: Readonly<{ profile: Profile }>) {
  const t = useT();
  const [baseline, setBaseline] = useState(profile);
  const [form, setForm] = useState(profile);
  const calendar = useMeetingCalendar(
    { ...form, enabled: profile.enabled },
    baseline,
  );
  const patch = usePatchSchedulingProfile();
  const toast = useToast();
  const titleRef = useRef<HTMLSpanElement>(null);
  const { provider, ready, selected, calendars, connections } = calendar;
  if (connections.isPending)
    return (
      <Panel title={t("scheduling.calendarTitle")}>
        <PendingBody label={t("scheduling.calendarTitle")} />
      </Panel>
    );
  if (calendar.providers.length === 0) return null;
  const connection = calendar.providers.find(
    (item) => item.provider === provider,
  );
  const dirty =
    JSON.stringify(calendarFields(form)) !==
      JSON.stringify(calendarFields(baseline)) ||
    (provider !== "" && provider !== baseline.provider);
  const writable =
    ready && !!selected?.writable && !calendars.isFetching && !!provider;
  const save = () => {
    if (!selected) return;
    patch.mutate(
      {
        provider,
        calendar_id: selected.id,
        blocking_calendars: calendar.blocking,
      },
      {
        onSuccess: (value) => {
          // The foot unmounts with the draft, so focus moves to the title first.
          titleRef.current?.focus();
          setBaseline(value);
          setForm(value);
          toast.show(t("settings.saved"));
        },
      },
    );
  };
  return (
    <Panel
      title={
        <span ref={titleRef} tabIndex={-1}>
          {t("scheduling.calendarTitle")}
        </span>
      }
      actions={
        dirty ? (
          <div className="meeting-card-save">
            <ErrorLine error={patch.error} />
            <Button
              variant="primary"
              disabled={!writable}
              pending={patch.isPending}
              onClick={save}
            >
              {t("scheduling.saveCalendar")}
            </Button>
          </div>
        ) : undefined
      }
    >
      {connection && (
        <PanelBody>
          <ProviderList>
            <CalendarConnectionRow connection={connection} ready={ready} />
          </ProviderList>
        </PanelBody>
      )}
      <PanelBody>
        <SettingList>
          {calendar.providers.length > 1 && (
            <SettingRow
              label={t("scheduling.provider")}
              control={(control) => (
                <Select
                  {...control}
                  value={provider}
                  options={[
                    { value: "", label: t("scheduling.chooseProvider") },
                    ...calendar.providers.map((item) => ({
                      value: item.provider,
                      label: calendarName(t, item.provider),
                    })),
                  ]}
                  onChange={(value) => {
                    if (
                      value === "" ||
                      value === "gcal" ||
                      value === "graphcal"
                    )
                      setForm({
                        ...form,
                        provider: value,
                        calendar_id: "primary",
                        blocking_calendars: [],
                      });
                  }}
                />
              )}
            />
          )}
          {provider !== "" && ready && (
            <CalendarChoices
              calendar={calendar}
              onCalendar={(value) =>
                setForm({
                  ...form,
                  provider,
                  calendar_id: value,
                  blocking_calendars: calendar.blocking.filter(
                    (id) => id !== value,
                  ),
                })
              }
              onBlocking={(value) =>
                setForm({
                  ...form,
                  provider,
                  calendar_id: calendar.calendarID,
                  blocking_calendars: value,
                })
              }
            />
          )}
          {provider !== "" && !ready && <KeptChoices profile={baseline} />}
        </SettingList>
        {ready && !!calendars.data && !selected && (
          <ErrorLine>{t("scheduling.missingSavedCalendar")}</ErrorLine>
        )}
        {!!form.provider && provider !== form.provider && (
          <p className="t-caption">{t("scheduling.providerChanged")}</p>
        )}
      </PanelBody>
    </Panel>
  );
}

function calendarFields(profile: Profile) {
  return {
    provider: profile.provider,
    calendar_id: profile.calendar_id,
    blocking_calendars: profile.blocking_calendars ?? [],
  };
}

function CalendarConnectionRow({
  connection,
  ready,
}: Readonly<{ connection: CaptureConnection; ready: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const now = useNow(60_000);
  const reconnect = useConnectAccount();
  const expired = connection.status === "reauth_required";
  const readOnly = !expired && missingCalendarWriteGrant(connection);
  const syncedAt = connection.last_synced_at
    ? Date.parse(connection.last_synced_at)
    : Number.NaN;
  const openConnections = () =>
    navigate({ screen: "settings", id: "connections" });
  let status: { tone: "success" | "warning"; label: string } = {
    tone: "success",
    label: t("connectors.health.connected"),
  };
  if (expired)
    status = { tone: "warning", label: t("scheduling.reconnectNeeded") };
  else if (readOnly)
    status = { tone: "warning", label: t("scheduling.readOnlyBadge") };
  else if (connection.status === "error")
    status = { tone: "warning", label: t("connectors.health.attention") };
  const provider = connection.provider;
  return (
    <ProviderRow
      mark={<ProviderMark providerKey={vendorOf(provider) ?? provider} />}
      name={calendarName(t, provider)}
      status={status}
      account={connection.account_label ?? undefined}
      facts={
        expired ? (
          <span>{t("scheduling.expiredRow")}</span>
        ) : (
          <>
            <Chip icon={CalendarClock} dense>
              {t("scheduling.readsBusy")}
            </Chip>
            {!readOnly && (
              <Chip icon={Send} dense>
                {t("scheduling.sendsInvites")}
              </Chip>
            )}
            {readOnly && <span>{t("scheduling.readOnlyCalendar")}</span>}
            {!readOnly && !Number.isNaN(syncedAt) && (
              <span>
                {t("scheduling.syncedAgo", {
                  ago: formatElapsed(now - syncedAt, t, locale),
                })}
              </span>
            )}
          </>
        )
      }
      action={
        ready ? (
          <Button onClick={openConnections}>
            {t("scheduling.openConnections")}
          </Button>
        ) : (
          <Button
            variant="primary"
            pending={reconnect.isPending}
            onClick={() =>
              (provider === "gcal" || provider === "graphcal") &&
              reconnect.mutate({ provider, returnTo: "meetings" })
            }
          >
            {t("scheduling.reconnect")}
          </Button>
        )
      }
      menu={
        ready ? undefined : (
          <OverflowMenu label={calendarName(t, provider)}>
            <Button onClick={openConnections}>
              {t("scheduling.openConnections")}
            </Button>
          </OverflowMenu>
        )
      }
    >
      <ConnectRefusal connect={reconnect} />
    </ProviderRow>
  );
}

function CalendarChoices({
  calendar,
  onCalendar,
  onBlocking,
}: Readonly<{
  calendar: ReturnType<typeof useMeetingCalendar>;
  onCalendar: (id: string) => void;
  onBlocking: (ids: string[]) => void;
}>) {
  const t = useT();
  const { calendarID, blocking } = calendar;
  return (
    <QueryGate query={calendar.calendars} pendingLabel={t("common.loading")}>
      {(calendars) =>
        calendars.some((item) => item.writable) ? (
          <>
            <SettingRow
              label={t("scheduling.eventCalendar")}
              description={t("scheduling.eventCalendarHelp")}
              control={(control) => (
                <Select
                  {...control}
                  value={
                    calendarID === "primary"
                      ? (calendars.find((item) => item.primary)?.id ??
                        calendarID)
                      : calendarID
                  }
                  options={calendars
                    .filter((item) => item.writable)
                    .map((item) => ({ value: item.id, label: item.name }))}
                  onChange={onCalendar}
                />
              )}
            />
            <SettingRow
              label={t("scheduling.blocking")}
              description={t("scheduling.blockingRowHelp")}
              control={(control) => (
                <MultiSelect
                  {...control}
                  values={blocking}
                  placeholder={t("scheduling.noneChosen")}
                  options={calendars
                    .filter(
                      (item) =>
                        item.id !== calendarID &&
                        !(calendarID === "primary" && item.primary),
                    )
                    .map((item) => ({ value: item.id, label: item.name }))}
                  onChange={onBlocking}
                />
              )}
            />
          </>
        ) : (
          <p>{t("scheduling.noWritableCalendar")}</p>
        )
      }
    </QueryGate>
  );
}

// The calendars cannot be listed while the connection is down, so the saved
// choices are stated rather than drawn as controls nobody can open.
function KeptChoices({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const plural = usePlural();
  const blocking = profile.blocking_calendars ?? [];
  return (
    <>
      <SettingRow
        label={t("scheduling.eventCalendar")}
        description={t("scheduling.keptAsSaved")}
        value={t(
          profile.calendar_id === "primary"
            ? "scheduling.mainCalendar"
            : "scheduling.savedCalendar",
        )}
        control={null}
      />
      <SettingRow
        label={t("scheduling.blocking")}
        description={t("scheduling.keptAsSaved")}
        value={
          blocking.length === 0
            ? t("scheduling.noneChosen")
            : plural("scheduling.calendarCount", blocking.length, {
                count: String(blocking.length),
              })
        }
        control={null}
      />
    </>
  );
}
