// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { ResolvedPassportChip } from "../../components/passportchip";
import { type Fact, FactList } from "../../design-system/factlist";
import { EvidenceChip, FieldDiff, toEvidence } from "../../design-system/trust";
import {
  type PluralTranslator,
  type Translator,
  usePlural,
  useT,
} from "../../i18n";
import { roleLabel } from "../../lib/roles.queries";
import { humanizeToken } from "./audit";
import {
  type AuditLogEntry,
  type AuthorizationRule,
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
            value: (
              <ResolvedPassportChip
                passportId={entry.passport_id}
                agentClient={entry.agent_client}
              />
            ),
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
      {keys.length > 0 && <AuditFieldFacts entry={entry} keys={keys} />}
      <FactList facts={facts} />
    </div>
  );
}

// A change reads as before and after; a creation or a removal as the one value
// the record held.
function AuditFieldFacts({
  entry,
  keys,
}: Readonly<{ entry: AuditLogEntry; keys: string[] }>) {
  const shape = changeShape(entry);
  const value = (key: string): ReactNode => {
    if (shape === "changed") {
      return (
        <FieldDiff
          oldValue={fieldValue(entry.before, key)?.text ?? null}
          newValue={fieldValue(entry.after, key)?.text ?? null}
        />
      );
    }
    const held = shape === "removed" ? entry.before : entry.after;
    return <AuditValue value={fieldValue(held, key)} />;
  };
  return (
    <FactList
      facts={keys.map((key) => ({
        key,
        term: humanizeToken(key),
        value: value(key),
      }))}
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
