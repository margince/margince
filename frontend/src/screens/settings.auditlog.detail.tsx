import { type ReactNode, useId } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge, Disclosure, Modal } from "../design-system/atoms";
import { FactList } from "../design-system/factlist";
import { Heading } from "../design-system/heading";
import {
  EvidenceChip,
  FieldDiff,
  PassportChip,
  toEvidence,
} from "../design-system/trust";
import { stable } from "../format/collate";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import { ActorTag } from "./audit";
import {
  AUDIT_ACTION_LABEL,
  auditActionTone,
  sentenceCaseToken,
} from "./settings.auditlog.vocab";

type AuditLogEntry = components["schemas"]["AuditLogEntry"];

// The union of before/after keys for one row's diff: a key present on neither
// side is never shown, so the drawer only displays fields the change touched.
// `stable`, because these are the record's own column names and their order
// must not depend on the reader's UI language.
function diffKeys(
  before: AuditLogEntry["before"],
  after: AuditLogEntry["after"],
): string[] {
  const keys = new Set([
    ...Object.keys(before ?? {}),
    ...Object.keys(after ?? {}),
  ]);
  return [...keys].sort(stable);
}

// An absent key reads as an explicit null through FieldDiff's honest empty
// marker; an object value is JSON, never "[object Object]".
function diffValue(
  rec: AuditLogEntry["before"] | AuditLogEntry["after"],
  key: string,
): string | null {
  const value = rec?.[key];
  if (value === null || value === undefined) {
    return null;
  }
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}

function DetailSection({
  title,
  children,
}: Readonly<{ title: string; children: ReactNode }>) {
  return (
    <section className="audit-log-section">
      <Heading size="xsmall" as="h3">
        {title}
      </Heading>
      {children}
    </section>
  );
}

function AuditLogDiff({ entry }: Readonly<{ entry: AuditLogEntry }>) {
  const t = useT();
  const keys = diffKeys(entry.before, entry.after);
  if (keys.length === 0) {
    return <p className="t-caption">{t("settings.auditLog.noDiff")}</p>;
  }
  return (
    <div className="audit-log-diff">
      {keys.map((key) => (
        <div key={key} className="audit-log-diff-line">
          {/* The field in words; the stored column name stays on hover for
              whoever has to trace it. */}
          <span className="t-label" title={key}>
            {sentenceCaseToken(key)}
          </span>
          <FieldDiff
            oldValue={diffValue(entry.before, key)}
            newValue={diffValue(entry.after, key)}
          />
        </div>
      ))}
    </div>
  );
}

// Exact identifiers for whoever has to quote this entry elsewhere: the UTC
// instant, the ids, and the on-behalf-of principal as recorded.
function technicalFacts(entry: AuditLogEntry, t: ReturnType<typeof useT>) {
  const facts = [
    {
      key: "time",
      term: t("settings.auditLog.exactTime"),
      value: entry.occurred_at,
    },
    { key: "entry", term: t("settings.auditLog.entryId"), value: entry.id },
    {
      key: "record",
      term: t("settings.auditLog.recordId"),
      value: entry.entity_id,
    },
    {
      key: "type",
      term: t("settings.auditLog.recordType"),
      value: entry.entity_type,
    },
    {
      key: "actor",
      term: t("settings.auditLog.actorId"),
      value: entry.actor_id,
    },
    {
      key: "batch",
      term: t("settings.auditLog.batchId"),
      value: entry.batch_id,
    },
    {
      key: "onBehalf",
      term: t("settings.auditLog.onBehalf"),
      value: entry.on_behalf_of_name ?? entry.on_behalf_of,
    },
  ];
  return facts.flatMap((fact) =>
    fact.value ? [{ ...fact, value: <code>{fact.value}</code> }] : [],
  );
}

// One change, opened beside the table: who did it and through what, what it
// changed, and on which permission it was allowed.
export function AuditLogDetail({
  entry,
  meUserId,
  onClose,
}: Readonly<{
  entry: AuditLogEntry;
  meUserId?: string;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const titleId = useId();
  const evidence = toEvidence(entry.evidence);
  return (
    <Modal open onClose={onClose} labelledBy={titleId} placement="right">
      <Heading size="large" id={titleId} className="modal-title">
        {t("settings.auditLog.detailTitle")}
      </Heading>
      <p className="audit-log-detail-head">
        <Badge tone={auditActionTone(entry.action)}>
          {t(AUDIT_ACTION_LABEL[entry.action])}
        </Badge>
        <time className="t-caption" dateTime={entry.occurred_at}>
          {formatDateTime(entry.occurred_at, locale, recordZone)}
        </time>
      </p>
      <DetailSection title={t("settings.auditLog.who")}>
        <div className="audit-log-who">
          <ActorTag entry={entry} meUserId={meUserId} />
          {entry.passport_id && <PassportChip id={entry.passport_id} />}
        </div>
      </DetailSection>
      <DetailSection title={t("settings.auditLog.changed")}>
        <AuditLogDiff entry={entry} />
      </DetailSection>
      {(entry.authorization_rule || evidence) && (
        <DetailSection title={t("settings.auditLog.why")}>
          {entry.authorization_rule && (
            <p className="t-caption">
              {t("settings.auditLog.rule")}:{" "}
              <code>{entry.authorization_rule}</code>
            </p>
          )}
          {evidence && <EvidenceChip evidence={evidence} />}
        </DetailSection>
      )}
      <Disclosure summary={t("settings.auditLog.technical")}>
        <FactList facts={technicalFacts(entry, t)} />
      </Disclosure>
    </Modal>
  );
}
