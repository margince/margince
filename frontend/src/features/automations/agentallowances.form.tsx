// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Button, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { describedByIds } from "../../design-system/describedby";
import { Eyebrow } from "../../design-system/eyebrow";
import { PanelBody } from "../../design-system/panel";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { boundsText } from "../../format/bounds";
import { formatNumber } from "../../format/format";
import { formatInterval } from "../../format/interval";
import { type FieldLock, lockFor } from "../../format/settinglocks";
import { type Locale, useT } from "../../i18n";
import {
  AGENT_ALLOWANCE_BOUNDS,
  type Allowances,
  COUNTS,
  type Counts,
  changesOf,
  chosenWindow,
  type Draft,
  defaultsDraft,
  EMPTY_ROLE_DRAFT,
  type Fallback,
  fallbackOf,
  hasRefusal,
  hasRoleRefusal,
  parseDraft,
  type RoleAllowance,
  type RoleAllowanceSet,
  type RoleDrafts,
  roleChangesOf,
  roleDraftOf,
} from "./agentallowances.draft";
import { keepLocked, RolesForm, roleDiffLines } from "./agentallowances.roles";

// The edit form of the allowances card: the installation value, the window,
// every role's values, and what a save will do.

type Locks = components["schemas"]["InstallationSettings"]["locks"];
type AllowancePatch = Partial<Record<Counts | "agent_window_hours", number>> & {
  agent_role_allowances?: RoleAllowanceSet[];
  agent_allowances_to_defaults?: boolean;
};

// The lengths an admin picks from. The one already saved is added when it is
// not among them, so a value set through the API still shows and keeps.
const WINDOW_PRESETS = [1, 6, 12, 24, 168] as const;

export function InstallationHead() {
  const t = useT();
  return (
    <PanelBody className="form-stack">
      <Eyebrow as="h3">{t("agentAllowances.installation.title")}</Eyebrow>
      <p className="t-caption">{t("agentAllowances.installation.sub")}</p>
    </PanelBody>
  );
}

export function AllowanceForm({
  allowances,
  locks,
  draft,
  roleDrafts,
  toDefaults,
  locale,
  busy,
  onDraft,
  onRoleDrafts,
  onToDefaults,
  onSave,
  onCancel,
}: Readonly<{
  allowances: Allowances;
  locks: Locks;
  draft: Draft;
  roleDrafts: RoleDrafts;
  // Use defaults was pressed and nothing typed since: the save removes the
  // stored values instead of saving the shipped numbers as a choice.
  toDefaults: boolean;
  locale: Locale;
  busy: boolean;
  onDraft: (next: Draft) => void;
  onRoleDrafts: (next: RoleDrafts) => void;
  onToDefaults: () => void;
  onSave: (patch: AllowancePatch) => void;
  onCancel: () => void;
}>) {
  const t = useT();
  const lockOf = (property: keyof Draft): FieldLock =>
    lockFor(locks, `installation.${property}`, t);
  const windowLock = lockOf("agent_window_hours");
  const roleLockOf = (property: Counts): FieldLock =>
    lockFor(locks, `installation.${roleMapProperty(property)}`, t);
  const parsed = parseDraft(draft);
  const roles = roleChangesOf(allowances, roleDrafts);
  const window = changesOf(allowances, parsed).agent_window_hours;
  const patch: AllowancePatch = toDefaults
    ? {
        agent_allowances_to_defaults: true,
        ...(window === undefined ? {} : { agent_window_hours: window }),
      }
    : {
        ...changesOf(allowances, parsed),
        ...(roles === undefined ? {} : { agent_role_allowances: roles }),
      };
  const fallback = (role: RoleAllowance, property: Counts) =>
    fallbackOf(allowances, role, property, {
      toDefaults,
      installation: parsed,
      locked: (count) => lockOf(count).disabled,
    });
  const changed = Object.keys(patch).length > 0;
  const refused = hasRefusal(parsed) || hasRoleRefusal(roleDrafts);
  return (
    <>
      <InstallationHead />
      <SettingList bleed="settings">
        {COUNTS.map((row) => {
          const refused = parsed[row.property] === null;
          const lock = lockOf(row.property);
          return (
            <SettingRow
              key={row.property}
              label={t(`agentAllowances.${row.copy}.label`)}
              description={t(`agentAllowances.${row.copy}.help`)}
              control={(control) => (
                <div className="settingrow-measure numbersetting">
                  <TextInput
                    {...control}
                    className="t-num"
                    data-testid={`allowance-${row.property}`}
                    inputMode="numeric"
                    aria-describedby={describedByIds(
                      control["aria-describedby"],
                      `${control.id}-note`,
                    )}
                    aria-invalid={refused ? true : undefined}
                    value={draft[row.property]}
                    disabled={busy || lock.disabled}
                    onChange={(e) =>
                      onDraft({ ...draft, [row.property]: e.target.value })
                    }
                  />
                  <span id={`${control.id}-note`} className="t-caption">
                    {lock.disabled
                      ? lock.reason
                      : refused
                        ? t(
                            "operations.refusal.count",
                            boundsText(
                              AGENT_ALLOWANCE_BOUNDS[row.property],
                              locale,
                            ),
                          )
                        : t("agentAllowances.default", {
                            value: formatNumber(
                              allowances.defaults[row.property],
                              locale,
                            ),
                          })}
                  </span>
                </div>
              )}
            />
          );
        })}
        <SettingRow
          label={t("agentAllowances.window.label")}
          description={
            windowLock.disabled
              ? windowLock.reason
              : t("agentAllowances.window.help")
          }
          control={(control) => (
            <Select
              {...control}
              value={draft.agent_window_hours}
              disabled={busy || windowLock.disabled}
              options={windowOptions(allowances, locale)}
              onChange={(value) =>
                onDraft({ ...draft, agent_window_hours: value })
              }
              testId="allowance-agent_window_hours"
            />
          )}
        />
      </SettingList>
      <RolesForm
        allowances={allowances}
        drafts={roleDrafts}
        busy={busy}
        lockOf={roleLockOf}
        fallback={fallback}
        onDrafts={onRoleDrafts}
      />
      <PanelBody className="form-stack">
        {changed && (
          <Diff
            allowances={allowances}
            patch={patch}
            roleDrafts={roleDrafts}
            locale={locale}
            fallback={fallback}
            toDefaults={toDefaults}
          />
        )}
        <div className="form-actions">
          <Button
            variant="primary"
            disabled={!changed || refused}
            pending={busy}
            onClick={() => onSave(patch)}
          >
            {t("agentAllowances.save")}
          </Button>
          <Button disabled={busy} onClick={onCancel}>
            {t("agentAllowances.cancel")}
          </Button>
          <Button
            disabled={busy}
            onClick={() => {
              onDraft(
                defaultsDraft(
                  allowances,
                  (property) => lockOf(property).disabled,
                ),
              );
              onRoleDrafts(
                defaultRoleDrafts(allowances, roleDrafts, roleLockOf),
              );
              onToDefaults();
            }}
          >
            {t("agentAllowances.useDefaults")}
          </Button>
        </div>
      </PanelBody>
    </>
  );
}

// Every role's values given up: each role inherits, and Admin takes its preset.
function defaultRoleDrafts(
  allowances: Allowances,
  drafts: RoleDrafts,
  lockOf: (property: Counts) => FieldLock,
): RoleDrafts {
  return Object.fromEntries(
    allowances.roles.map((role) => [
      role.role_key,
      keepLocked(
        EMPTY_ROLE_DRAFT,
        drafts[role.role_key] ?? roleDraftOf(role),
        lockOf,
      ),
    ]),
  );
}

// roleMapProperty is the setting a count's role values are stored under.
export function roleMapProperty(property: Counts): string {
  return property.replace("agent_", "agent_role_");
}

function windowOptions(allowances: Allowances, locale: Locale) {
  const hours = new Set<number>([...WINDOW_PRESETS, chosenWindow(allowances)]);
  return [...hours]
    .sort((a, b) => a - b)
    .map((value) => ({
      value: String(value),
      label: formatInterval(value * 3_600, locale),
    }));
}

// What a save would do, before it does it. A count binds within a minute; a
// window length waits for the next reset, and the line says so.
function Diff({
  allowances,
  patch,
  roleDrafts,
  locale,
  fallback,
  toDefaults,
}: Readonly<{
  allowances: Allowances;
  patch: AllowancePatch;
  roleDrafts: RoleDrafts;
  locale: Locale;
  fallback: (role: RoleAllowance, property: Counts) => Fallback;
  toDefaults: boolean;
}>) {
  const t = useT();
  const roleLines = toDefaults
    ? [t("agentAllowances.diff.toDefaults")]
    : roleDiffLines(allowances, roleDrafts, t, locale, fallback);
  const lines = COUNTS.flatMap((row) => {
    const next = patch[row.property];
    return next === undefined
      ? []
      : [
          t("agentAllowances.diff.count", {
            label: t(`agentAllowances.${row.copy}.label`),
            from: formatNumber(allowances[row.property], locale),
            to: formatNumber(next, locale),
          }),
        ];
  });
  const hours = patch.agent_window_hours;
  return (
    <Callout tone="warning" title={t("agentAllowances.diff.title")}>
      {lines.length > 0 && <p>{lines.join(" · ")}</p>}
      {roleLines.length > 0 && <p>{roleLines.join(" · ")}</p>}
      {lines.length + roleLines.length > 0 && (
        <p>{t("agentAllowances.diff.counts")}</p>
      )}
      {roleLines.length > 0 && <p>{t("agentAllowances.diff.roles")}</p>}
      {hours !== undefined && (
        <p>
          {t("agentAllowances.diff.window", {
            from: formatInterval(chosenWindow(allowances) * 3_600, locale),
            to: formatInterval(hours * 3_600, locale),
          })}
        </p>
      )}
    </Callout>
  );
}
