// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../../api/schema";
import { useUnsavedGuard } from "../../components/unsaved";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { formatDateTime, formatNumber } from "../../format/format";
import { formatInterval } from "../../format/interval";
import { lockFor } from "../../format/settinglocks";
import { UTC_ZONE } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemMessageOf } from "../../lib/problem";
import { useInstallationSettings } from "../../lib/uploadlimit";
import { useUpdateInstallationSettings } from "../sign-in/installation-settings";
import {
  type Allowances,
  COUNTS,
  type Counts,
  chosenWindow,
  type Draft,
  draftOf,
  type RoleAllowance,
  type RoleDrafts,
  roleDraftsOf,
} from "./agentallowances.draft";
import {
  AllowanceForm,
  InstallationHead,
  roleMapProperty,
} from "./agentallowances.form";
import { RolesReadout } from "./agentallowances.roles";
import { useResetLine } from "./agentusage";

type Locks = components["schemas"]["InstallationSettings"]["locks"];

// What one passport may do in each window: the values at rest, and an edit
// form behind a button. A count binds within a minute, a window length at the
// next reset.
export function AgentAllowancesCard() {
  const t = useT();
  const canRead = useCan("installation_settings", "read");
  const canManage = useCanWrite("installation_settings", "update");
  const query = useInstallationSettings(canRead);
  if (!canRead) {
    return null;
  }
  const title = t("agentAllowances.title");
  return (
    <Panel title={title}>
      <PanelBody className="form-stack">
        <PanelIntro>{t("agentAllowances.sub")}</PanelIntro>
        {!canManage && <PanelIntro>{t("operations.adminOnly")}</PanelIntro>}
      </PanelBody>
      <QueryGate query={query} pendingLabel={title}>
        {(settings) => {
          // A server older than this screen answers without them.
          if (settings.agent_allowances == null) {
            return null;
          }
          return (
            <AllowancesBody
              allowances={{
                ...settings.agent_allowances,
                // A server older than this screen answers without the roles.
                roles: settings.agent_allowances.roles ?? [],
              }}
              locks={settings.locks}
              canManage={canManage}
            />
          );
        }}
      </QueryGate>
    </Panel>
  );
}

// Edit's refusal when a boot source sets every allowance, each reason once:
// the form would open with nothing in it to change.
function lockedEditReason(locks: Locks, t: Translator): string | undefined {
  const properties = [
    ...COUNTS.map((row) => row.property),
    ...COUNTS.map((row) => roleMapProperty(row.property)),
    "agent_window_hours",
  ];
  const reasons = properties.map((property) => {
    const lock = lockFor(locks, `installation.${property}`, t);
    return lock.disabled ? lock.reason : null;
  });
  if (reasons.includes(null)) {
    return undefined;
  }
  return [...new Set(reasons)].join(" ");
}

function AllowancesBody({
  allowances,
  locks,
  canManage,
}: Readonly<{ allowances: Allowances; locks: Locks; canManage: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<Draft | null>(null);
  const [roleDrafts, setRoleDrafts] = useState<RoleDrafts>({});
  // The allowances the form opened on. A save sends what moved from these,
  // so a refresh mid-edit cannot turn another admin's new value into a change.
  const [openedOn, setOpenedOn] = useState<Allowances | null>(null);
  useUnsavedGuard(draft !== null);
  // A new allowance is the limit every usage bar is drawn against.
  const update = useUpdateInstallationSettings(() => {
    setDraft(null);
    for (const key of [["passports"], ["agent-usage"]]) {
      void queryClient.invalidateQueries({ queryKey: key });
    }
  });
  const resetLine = useResetLine();
  return (
    <>
      {draft === null ? (
        <>
          <AllowanceReadout allowances={allowances} locale={locale} />
          <RolesReadout allowances={allowances} />
        </>
      ) : (
        <AllowanceForm
          allowances={openedOn ?? allowances}
          locks={locks}
          draft={draft}
          roleDrafts={roleDrafts}
          locale={locale}
          busy={update.isPending}
          onDraft={setDraft}
          onRoleDrafts={setRoleDrafts}
          onSave={(patch) => update.mutate(patch)}
          onCancel={() => {
            setDraft(null);
            update.reset();
          }}
        />
      )}
      <PanelBody className="form-stack">
        <p className="t-caption">
          {t("agentAllowances.windowLine", {
            interval: formatInterval(
              allowances.agent_window_hours * 3_600,
              locale,
            ),
          })}
          {" · "}
          {resetLine(allowances.agent_window_resets_at)}
        </p>
        <PendingWindow allowances={allowances} />
        {draft === null && <DefaultsNote allowances={allowances} />}
        {draft === null && canManage && (
          <div className="card-actions">
            <Button
              reason={lockedEditReason(locks, t)}
              onClick={() => {
                update.reset();
                setDraft(draftOf(allowances));
                setRoleDrafts(roleDraftsOf(allowances.roles));
                setOpenedOn(allowances);
              }}
            >
              {t("agentAllowances.edit")}
            </Button>
          </div>
        )}
        {update.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("operations.updateFailed")}
          >
            {problemMessageOf(update.error, t)}
          </Callout>
        )}
        <p className="t-caption">
          {t("agentAllowances.burst", {
            calls: formatNumber(allowances.mcp_calls_per_minute, locale),
            streams: formatNumber(
              allowances.mcp_stream_opens_per_minute,
              locale,
            ),
          })}
        </p>
      </PanelBody>
    </>
  );
}

function AllowanceReadout({
  allowances,
  locale,
}: Readonly<{ allowances: Allowances; locale: Locale }>) {
  const t = useT();
  return (
    <>
      <InstallationHead />
      <StatStrip testId="allowance-readout">
        {COUNTS.map((row) => (
          <StatCard
            key={row.property}
            label={t(`agentAllowances.${row.copy}.label`)}
            value={formatNumber(allowances[row.property], locale)}
            detail={t(
              row.release
                ? "agentAllowances.note.release"
                : "agentAllowances.note.stop",
            )}
          />
        ))}
      </StatStrip>
    </>
  );
}

// A role is at its shipped value for a count when it holds none of its own.
// The preset's role also needs the preset to bind where it has one.
function atShippedValue(role: RoleAllowance, property: Counts): boolean {
  const { source } = role[property];
  if (source === "role" || source === "installation_kept") {
    return false;
  }
  // Sends have no preset; the preset's role is at its shipped value for the
  // other counts only while the preset binds.
  const hasPreset = role.holds_preset && property !== "agent_egress_per_window";
  return !hasPreset || source === "admin_preset";
}

// Says in words whether any value is off its default, so a reader of the plain
// numbers knows if they are looking at what ships. A role's own value is one.
function DefaultsNote({ allowances }: Readonly<{ allowances: Allowances }>) {
  const t = useT();
  const atDefaults =
    COUNTS.every(
      (row) =>
        allowances[row.property] === allowances.defaults[row.property] &&
        allowances.roles.every((role) => atShippedValue(role, row.property)),
    ) && chosenWindow(allowances) === allowances.defaults.agent_window_hours;
  return (
    <Callout
      tone="info"
      title={t(
        atDefaults
          ? "agentAllowances.state.defaults"
          : "agentAllowances.state.changed",
      )}
    >
      {t("agentAllowances.state.body")}
    </Callout>
  );
}

// A saved window length that has not taken over yet. Said out loud, because
// the readout still shows the length in force and a reader would think the
// save did nothing.
function PendingWindow({ allowances }: Readonly<{ allowances: Allowances }>) {
  const t = useT();
  const { locale } = useLocale();
  const hours = allowances.agent_window_next_hours;
  const from = allowances.agent_window_next_starts_at;
  if (hours == null || from == null) {
    return null;
  }
  return (
    <Callout tone="info" title={t("agentAllowances.pending.title")}>
      {t("agentAllowances.pending.body", {
        interval: formatInterval(hours * 3_600, locale),
        date: formatDateTime(from, locale, UTC_ZONE),
      })}
    </Callout>
  );
}
