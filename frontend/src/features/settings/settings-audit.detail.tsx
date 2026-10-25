// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { ResolvedPassportChip } from "../../components/passportchip";
import { type Fact, FactList } from "../../design-system/factlist";
import { EvidenceChip, FieldDiff, toEvidence } from "../../design-system/trust";
import {
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { roleLabel } from "../../lib/roles.queries";
import { precisionOf } from "../records/history.logic";
import { type HistoryValueCtx, historyValue } from "../records/historyvalues";
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
  const { locale } = useLocale();
  const zone = useRecordZone();
  const ctx: HistoryValueCtx = { currency: null, locale, zone };
  const value = (key: string): ReactNode => {
    if (shape === "changed") {
      return (
        <FieldDiff
          oldValue={auditValue(entry.before, key, ctx)?.text ?? null}
          newValue={auditValue(entry.after, key, ctx)?.text ?? null}
        />
      );
    }
    const held = shape === "removed" ? entry.before : entry.after;
    return <AuditValue value={auditValue(held, key, ctx)} />;
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

// An activity's time reads as the History tab reads it, by the precision stored
// beside it; every other value as stored.
function auditValue(
  image: AuditLogEntry["before"],
  key: string,
  ctx: HistoryValueCtx,
): FieldValue | null {
  const held = fieldValue(image, key);
  const precision = precisionOf(image ?? {}, key);
  if (!held || held.structured || !precision) {
    return held;
  }
  return {
    text: historyValue(key, held.text, ctx, precision) ?? held.text,
    structured: false,
  };
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
