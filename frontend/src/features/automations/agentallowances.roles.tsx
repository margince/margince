// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Badge, Button, TextInput } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Eyebrow } from "../../design-system/eyebrow";
import { PanelBody } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import {
  ADMIN_ROLE,
  type Allowances,
  COUNTS,
  type Counts,
  EMPTY_ROLE_DRAFT,
  parseRoleDraft,
  type RoleAllowance,
  type RoleDraft,
  type RoleDrafts,
  roleDraftOf,
} from "./agentallowances.draft";
import "../settings/settings-agents.css";

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
  // Calls first, as the readout leads with the ceiling every count sits under.
  return [...COUNTS].reverse().map((row) => ({
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

function inheritedValue(
  allowances: Allowances,
  property: Counts,
  locale: Locale,
): string {
  return formatNumber(allowances[property], locale);
}

export function RolesForm({
  allowances,
  drafts,
  busy,
  onDrafts,
}: Readonly<{
  allowances: Allowances;
  drafts: RoleDrafts;
  busy: boolean;
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
      const refused = parseRoleDraft(draft)[property] === null;
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
          placeholder={t("agentAllowances.roles.inherits", {
            value: inheritedValue(allowances, property, locale),
          })}
          value={draft[property]}
          disabled={busy}
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
          disabled={busy || sameAsEmpty(draftOf(role))}
          onClick={() => set(role, EMPTY_ROLE_DRAFT)}
        >
          {t("agentAllowances.roles.useInstallation")}
        </Button>
      ),
    },
  ];
  const refused = Object.values(drafts).some((draft) =>
    Object.values(parseRoleDraft(draft)).includes(null),
  );
  return (
    <>
      <RolesHead editing />
      {allowances.roles.some((role) => role.role_key === ADMIN_ROLE) && (
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
      {refused && (
        <PanelBody>
          <p className="t-caption" role="alert">
            {t("agentAllowances.roles.refusal")}
          </p>
        </PanelBody>
      )}
    </>
  );
}

function sameAsEmpty(draft: RoleDraft): boolean {
  return COUNTS.every(({ property }) => draft[property].trim() === "");
}

// One line per role count a save moves, for the callout that says what a save
// will do. An empty box reads as the installation value.
export function roleDiffLines(
  allowances: Allowances,
  drafts: RoleDrafts,
  t: Translator,
  locale: Locale,
): string[] {
  const shown = (text: string) =>
    text.trim() === ""
      ? t("agentAllowances.diff.inherited")
      : formatNumber(Number(text), locale);
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
              from: shown(before[row.property]),
              to: shown(after[row.property]),
            }),
          ],
    );
  });
}
