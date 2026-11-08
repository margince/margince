// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Badge, Button, TextInput } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Eyebrow } from "../../design-system/eyebrow";
import { PanelBody } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import type { FieldLock } from "../../format/settinglocks";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import {
  type Allowances,
  COUNTS,
  type Counts,
  EMPTY_ROLE_DRAFT,
  type Fallback,
  hasRoleRefusal,
  INSTALLATION_ROLE_DRAFT,
  parseRoleCell,
  type RoleAllowance,
  type RoleDraft,
  type RoleDrafts,
  roleDraftOf,
} from "./agentallowances.draft";
import "./agentallowances.css";

// What each role gives per count, and where each value comes from: the role's
// own, the Admin preset, or the installation value it inherits.

function RoleName({ role }: Readonly<{ role: RoleAllowance }>) {
  const t = useT();
  return (
    <span className="agent-role-name">
      {role.role_name}{" "}
      {role.system && <Badge>{t("agentAllowances.roles.builtIn")}</Badge>}
    </span>
  );
}

function RolesHead({ editing }: Readonly<{ editing: boolean }>) {
  const t = useT();
  return (
    <PanelBody className="form-stack">
      <Eyebrow as="h3">{t("agentAllowances.roles.title")}</Eyebrow>
      <p className="t-caption">
        {t(
          editing
            ? "agentAllowances.roles.editHelp"
            : "agentAllowances.roles.rule",
        )}
      </p>
    </PanelBody>
  );
}

type CountRow = (typeof COUNTS)[number];

function countColumns(
  t: Translator,
  cell: (role: RoleAllowance, row: CountRow) => ReactNode,
): DataTableColumn<RoleAllowance>[] {
  // Calls first: the ceiling every other count sits under.
  const order = ["calls", "writes", "egress"];
  const rows = [...COUNTS].sort(
    (a, b) => order.indexOf(a.copy) - order.indexOf(b.copy),
  );
  return rows.map((row) => ({
    key: row.property,
    header: t(`agentAllowances.${row.copy}.label`),
    align: "end" as const,
    render: (role: RoleAllowance) => cell(role, row),
  }));
}

export function RolesReadout({
  allowances,
}: Readonly<{ allowances: Allowances }>) {
  const t = useT();
  const { locale } = useLocale();
  const columns: DataTableColumn<RoleAllowance>[] = [
    {
      key: "role",
      header: t("agentAllowances.roles.col.role"),
      grow: true,
      fold: "title",
      render: (role) => <RoleName role={role} />,
    },
    ...countColumns(t, (role, { property }) => {
      const { value, source } = role[property];
      return (
        <span className="agent-role-value" data-source={source}>
          <span className="t-num">{formatNumber(value, locale)}</span>
          <span className="t-caption">
            {t(`agentAllowances.roles.source.${source}`)}
          </span>
        </span>
      );
    }),
  ];
  return (
    <>
      <RolesHead editing={false} />
      <DataTable
        bleed
        fold
        label={t("agentAllowances.roles.title")}
        columns={columns}
        rows={[...allowances.roles]}
        rowKey={(role) => role.role_key}
        rowTestId={(role) => `role-allowance-${role.role_key}`}
      />
    </>
  );
}

// What an empty box gives, said in its placeholder. The server works out the
// value a count takes once the role's own is given up.
function emptyBoxSays(
  fallback: Fallback,
  t: Translator,
  locale: Locale,
): string {
  return t(
    fallback.source === "admin_preset"
      ? "agentAllowances.roles.presetValue"
      : "agentAllowances.roles.inherits",
    { value: formatNumber(fallback.value, locale) },
  );
}

// The draft "Use the installation value" leaves. For the preset's role that is
// the installation value saved, since an empty box would take the preset. A
// count a boot source locks keeps its box, so the save is not refused for it.
function installationDraft(
  role: RoleAllowance,
  current: RoleDraft,
  lockOf: (property: Counts) => FieldLock,
): RoleDraft {
  const target = role.holds_preset ? INSTALLATION_ROLE_DRAFT : EMPTY_ROLE_DRAFT;
  return keepLocked(target, current, lockOf);
}

// keepLocked is next with every locked count's box left as it is in current.
export function keepLocked(
  next: RoleDraft,
  current: RoleDraft,
  lockOf: (property: Counts) => FieldLock,
): RoleDraft {
  const out = { ...next };
  for (const { property } of COUNTS) {
    if (lockOf(property).disabled) {
      out[property] = current[property];
    }
  }
  return out;
}

export function RolesForm({
  allowances,
  drafts,
  busy,
  lockOf,
  fallback,
  onDrafts,
}: Readonly<{
  allowances: Allowances;
  drafts: RoleDrafts;
  busy: boolean;
  // The lock on one count's role values, keyed by the count's property.
  lockOf: (property: Counts) => FieldLock;
  // What an emptied box gives once the save being drafted lands.
  fallback: (role: RoleAllowance, property: Counts) => Fallback;
  onDrafts: (next: RoleDrafts) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const draftOf = (role: RoleAllowance): RoleDraft =>
    drafts[role.role_key] ?? roleDraftOf(role);
  const set = (role: RoleAllowance, next: RoleDraft) =>
    onDrafts({ ...drafts, [role.role_key]: next });
  const columns: DataTableColumn<RoleAllowance>[] = [
    {
      key: "role",
      header: t("agentAllowances.roles.col.role"),
      grow: true,
      fold: "title",
      render: (role) => <RoleName role={role} />,
    },
    ...countColumns(t, (role, { property, copy }) => {
      const draft = draftOf(role);
      const refused = parseRoleCell(property, draft[property]) === null;
      const label = t(`agentAllowances.${copy}.label`);
      return (
        <TextInput
          className="t-num agent-role-input"
          inputMode="numeric"
          aria-label={t("agentAllowances.roles.inputLabel", {
            kind: label,
            role: role.role_name,
          })}
          aria-invalid={refused ? true : undefined}
          data-testid={`role-${role.role_key}-${property}`}
          placeholder={emptyBoxSays(fallback(role, property), t, locale)}
          value={draft[property]}
          disabled={busy || lockOf(property).disabled}
          onChange={(e) => set(role, { ...draft, [property]: e.target.value })}
        />
      );
    }),
    {
      key: "inherit",
      header: t("agentAllowances.roles.useInstallation"),
      headerHidden: true,
      fold: "end",
      render: (role) => (
        <Button
          variant="link"
          disabled={
            busy ||
            sameDraft(
              draftOf(role),
              installationDraft(role, draftOf(role), lockOf),
            )
          }
          onClick={() =>
            set(role, installationDraft(role, draftOf(role), lockOf))
          }
        >
          {t("agentAllowances.roles.useInstallation")}
        </Button>
      ),
    },
  ];
  const refused = hasRoleRefusal(drafts);
  const locked = [
    ...new Set(
      COUNTS.map(({ property }) => lockOf(property))
        .filter((lock) => lock.disabled)
        .map((lock) => lock.reason),
    ),
  ];
  return (
    <>
      <RolesHead editing />
      {allowances.roles.some((role) => role.holds_preset) && (
        <PanelBody>
          <p className="t-caption">
            {t("agentAllowances.roles.preset", {
              calls: formatNumber(
                allowances.defaults.admin_calls_per_window,
                locale,
              ),
              changes: formatNumber(
                allowances.defaults.admin_writes_per_window,
                locale,
              ),
            })}
          </p>
        </PanelBody>
      )}
      <DataTable
        bleed
        fold
        label={t("agentAllowances.roles.title")}
        columns={columns}
        rows={[...allowances.roles]}
        rowKey={(role) => role.role_key}
        rowTestId={(role) => `role-allowance-${role.role_key}`}
      />
      {locked.length > 0 && (
        <PanelBody>
          <p className="t-caption">{locked.join(" ")}</p>
        </PanelBody>
      )}
      {refused && (
        <PanelBody>
          <ErrorLine>{t("agentAllowances.roles.refusal")}</ErrorLine>
        </PanelBody>
      )}
    </>
  );
}

function sameDraft(a: RoleDraft, b: RoleDraft): boolean {
  return COUNTS.every(
    ({ property }) => a[property].trim() === b[property].trim(),
  );
}

// One line per role count a save moves, for the callout that says what a save
// will do. An empty box reads as what it gives: the preset or the installation.
export function roleDiffLines(
  allowances: Allowances,
  drafts: RoleDrafts,
  t: Translator,
  locale: Locale,
  fallback: (role: RoleAllowance, property: Counts) => Fallback,
): string[] {
  const shown = (text: string, emptyGives: Fallback) => {
    const trimmed = text.trim();
    if (trimmed === "" && emptyGives.source === "admin_preset") {
      return t("agentAllowances.diff.preset");
    }
    return trimmed === "" || trimmed === "0"
      ? t("agentAllowances.diff.inherited")
      : formatNumber(Number(trimmed), locale);
  };
  return allowances.roles.flatMap((role) => {
    const before = roleDraftOf(role);
    const after = drafts[role.role_key] ?? before;
    return COUNTS.flatMap((row) =>
      before[row.property].trim() === after[row.property].trim()
        ? []
        : [
            t("agentAllowances.diff.role", {
              role: role.role_name,
              label: t(`agentAllowances.${row.copy}.label`),
              from: shown(before[row.property], role[row.property].if_unset),
              to: shown(after[row.property], fallback(role, row.property)),
            }),
          ],
    );
  });
}
