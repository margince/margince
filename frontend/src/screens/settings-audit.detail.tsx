// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { type Fact, FactList } from "../design-system/factlist";
import { EvidenceChip, FieldDiff, toEvidence } from "../design-system/trust";
import {
  type PluralTranslator,
  type Translator,
  usePlural,
  useT,
} from "../i18n";
import { humanizeToken } from "./audit";
import { ResolvedPassportChip } from "./passportchip";
import { roleLabel } from "./roles.queries";
import {
  type AuditLogEntry,
  type AuthorizationRule,
  type ChangeShape,
  changeShape,
  diffKeys,
  type FieldValue,
  fieldValue,
  parseAuthorizationRule,
  ROW_SCOPE_WORDS,
} from "./settings-audit.format";

export function AuditDetail({ entry }: Readonly<{ entry: AuditLogEntry }>) {
  const t = useT();
  const plural = usePlural();
  const keys = diffKeys(entry.before, entry.after);
  const evidence = toEvidence(entry.evidence);
  const facts: Fact[] = [
    ...(entry.authorization_rule
      ? [
          {
            key: "rule",
            term: t("settings.auditAllowedBy"),
            value: ruleWords(
              parseAuthorizationRule(entry.authorization_rule),
              t,
              plural,
            ),
          },
        ]
      : []),
    {
      key: "record",
      term: t("settings.auditRecordId"),
      value: <code className="auditlog-id">{entry.entity_id}</code>,
    },
    ...(entry.passport_id
      ? [
          {
            key: "passport",
            term: t("history.passport"),
            value: <ResolvedPassportChip passportId={entry.passport_id} />,
          },
        ]
      : []),
    ...(evidence
      ? [
          {
            key: "evidence",
            term: t("settings.auditEvidence"),
            value: <EvidenceChip evidence={evidence} />,
          },
        ]
      : []),
  ];
  return (
    <div className="auditlog-detail-body">
      {keys.length > 0 && <AuditFieldTable entry={entry} keys={keys} />}
      <FactList facts={facts} />
    </div>
  );
}

const SHAPE_HEADER = {
  created: "settings.auditColValue",
  changed: "settings.auditColChange",
  removed: "settings.auditColRemoved",
} as const satisfies Record<ChangeShape, string>;

function AuditFieldTable({
  entry,
  keys,
}: Readonly<{ entry: AuditLogEntry; keys: string[] }>) {
  const t = useT();
  const shape = changeShape(entry);
  const columns: DataTableColumn<string>[] = [
    {
      key: "field",
      header: t("settings.auditColField"),
      render: (key) => humanizeToken(key),
    },
    {
      key: "value",
      header: t(SHAPE_HEADER[shape]),
      render: (key) =>
        shape === "changed" ? (
          <FieldDiff
            oldValue={fieldValue(entry.before, key)?.text ?? null}
            newValue={fieldValue(entry.after, key)?.text ?? null}
          />
        ) : (
          <AuditValue
            value={fieldValue(
              shape === "created" ? entry.after : entry.before,
              key,
            )}
          />
        ),
    },
  ];
  return (
    <DataTable
      label={t("settings.auditChanges")}
      columns={columns}
      rows={keys}
      rowKey={(key) => key}
    />
  );
}

function AuditValue({ value }: Readonly<{ value: FieldValue | null }>) {
  const t = useT();
  if (!value) {
    return <span className="t-caption">{t("settings.auditNoValue")}</span>;
  }
  return value.structured ? (
    <code className="auditlog-value">{value.text}</code>
  ) : (
    <span className="auditlog-value">{value.text}</span>
  );
}

function ruleWords(
  rule: AuthorizationRule,
  t: Translator,
  plural: PluralTranslator,
): string {
  switch (rule.kind) {
    case "system":
      return t("settings.auditRuleSystem");
    case "dealRoom":
      return t("settings.auditRuleDealRoom");
    case "raw":
      return rule.text;
    case "policy": {
      const role = roleLabel(t);
      const scopeKey = ROW_SCOPE_WORDS[rule.scope];
      return [
        rule.roles.length > 0
          ? plural("settings.auditRuleRole", rule.roles.length, {
              roles: rule.roles.map((key) => role(key)).join(", "),
            })
          : "",
        t("settings.auditRuleGrant", {
          object: humanizeToken(rule.object),
          action: humanizeToken(rule.action),
        }),
        scopeKey ? t(scopeKey) : rule.scope,
      ]
        .filter(Boolean)
        .join(" · ");
    }
  }
}
