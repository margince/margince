// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarClock, Send } from "lucide-react";
import { type RefObject, useRef, useState } from "react";
import type { components } from "../api/schema";
import { Button, OverflowMenu, PendingBody } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody } from "../design-system/panel";
import { PanelNotices } from "../design-system/panelnotices";
import { PanelSave } from "../design-system/panelsave";
import { ProviderMark } from "../design-system/provider-mark";
import { ProviderList, ProviderRow } from "../design-system/providerrow";
import { QueryGate } from "../design-system/querystates";
import { Chip } from "../design-system/readings";
import { MultiSelect, Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { type StatusKind, statusTone } from "../design-system/statusbadge";
import { useToast } from "../design-system/toast";
import { vendorOf } from "../format/connectorprovider";
import { formatNumber } from "../format/format";
import { formatElapsed, useNow } from "../format/now";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import { navigate } from "../lib/router";
import { calendarName, useMeetingCalendar } from "./booking-calendar-state";
import { notConfiguredProvider, useConnectAccount } from "./connect-account";
import { missingCalendarWriteGrant } from "./connector-status";
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
  const titleRef = useRef<HTMLSpanElement>(null);
  const draft = useCalendarDraft(profile, titleRef);
  const { calendar, patch } = draft;
  const { provider, ready, connections } = calendar;
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
  return (
    <Panel
      title={
        <span ref={titleRef} tabIndex={-1}>
          {t("scheduling.calendarTitle")}
        </span>
      }
      actions={
        // A calendar that is not ready cannot be saved, and its row's Reconnect
        // is then the card's one primary.
        <PanelSave
          dirty={draft.dirty && ready}
          pending={patch.isPending}
          saveLabel={t("scheduling.saveCalendar")}
          onSave={draft.save}
          onDiscard={draft.discard}
        />
      }
    >
      {connection && (
        <PanelBody>
          <ProviderList>
            <CalendarConnectionRow connection={connection} ready={ready} />
          </ProviderList>
        </PanelBody>
      )}
      <CalendarFields draft={draft} />
      <PanelNotices
        refused={
          patch.isError
            ? { title: t("scheduling.calendarSaveFailed"), error: patch.error }
            : undefined
        }
      />
    </Panel>
  );
}

// The card's draft against what was last saved, and the one write for it. The
// title ref stays with the caller: a draft holding a ref could not be read in render.
function useCalendarDraft(
  profile: Profile,
  titleRef: RefObject<HTMLSpanElement | null>,
) {
  const t = useT();
  const [baseline, setBaseline] = useState(profile);
  const [form, setForm] = useState(profile);
  const calendar = useMeetingCalendar(
    { ...form, enabled: profile.enabled },
    baseline,
  );
  const patch = usePatchSchedulingProfile();
  const toast = useToast();
  const { provider, ready, selected, calendars } = calendar;
  const dirty =
    JSON.stringify(calendarFields(form)) !==
      JSON.stringify(calendarFields(baseline)) ||
    (provider !== "" && provider !== baseline.provider);
  const writable =
    ready && !!selected?.writable && !calendars.isFetching && !!provider;
  const save = () => {
    // PanelSave has no refused state yet; the card says why beside the choice.
    if (!selected || !writable) return;
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
  const discard = () => {
    patch.reset();
    setForm(baseline);
  };
  return {
    baseline,
    form,
    setForm,
    calendar,
    patch,
    dirty,
    save,
    discard,
  };
}

/** Which calendar takes the bookings and which ones block time. */
function CalendarFields({
  draft,
}: Readonly<{ draft: ReturnType<typeof useCalendarDraft> }>) {
  const t = useT();
  const { form, setForm, calendar } = draft;
  const { provider, ready, selected, calendars } = calendar;
  return (
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
                  if (value === "" || value === "gcal" || value === "graphcal")
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
        {provider !== "" && !ready && <KeptChoices profile={draft.baseline} />}
      </SettingList>
      {ready && !!calendars.data && !selected && (
        <ErrorLine>{t("scheduling.missingSavedCalendar")}</ErrorLine>
      )}
      {!!form.provider && provider !== form.provider && (
        <p className="t-caption">{t("scheduling.providerChanged")}</p>
      )}
    </PanelBody>
  );
}

function calendarFields(profile: Profile) {
  return {
    provider: profile.provider,
    calendar_id: profile.calendar_id,
    blocking_calendars: profile.blocking_calendars ?? [],
  };
}

// Expired and read-only are one state to a reader: the reason line says which.
function calendarRowStanding(connection: CaptureConnection, t: Translator) {
  const expired = connection.status === "reauth_required";
  const readOnly = !expired && missingCalendarWriteGrant(connection);
  const kind: StatusKind =
    expired || readOnly || connection.status === "error" ? "attention" : "done";
  let reason: string | undefined;
  if (expired) reason = t("scheduling.expiredRow");
  else if (readOnly) reason = t("scheduling.readOnlyCalendar");
  return {
    status: {
      tone: statusTone(kind),
      label: t(
        kind === "done"
          ? "connectors.health.connected"
          : "connectors.health.attention",
      ),
    },
    reason,
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
  const { status, reason } = calendarRowStanding(connection, t);
  const provider = connection.provider;
  return (
    <ProviderRow
      mark={<ProviderMark providerKey={vendorOf(provider) ?? provider} />}
      name={calendarName(t, provider)}
      status={status}
      account={connection.account_label ?? undefined}
      reason={reason}
      facts={
        expired ? undefined : (
          <>
            <Chip icon={CalendarClock} dense>
              {t("scheduling.readsBusy")}
            </Chip>
            {!readOnly && (
              <Chip icon={Send} dense>
                {t("scheduling.sendsInvites")}
              </Chip>
            )}
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
  const { locale } = useLocale();
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
                count: formatNumber(blocking.length, locale),
              })
        }
        control={null}
      />
    </>
  );
}
