// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  EntityRef,
  missLabel,
  useEntityName,
  useRosterName,
} from "../../components/entityref";
import {
  accessLabel,
  type RecordGrant,
  type RecordType,
  SubjectKindIcon,
} from "../../components/share.parts";
import { Badge, Button } from "../../design-system/atoms";
import { PanelRow } from "../../design-system/panel";
import { formatDateAbbrev } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";

// Revoke and the grant toast say whose access to what, so an owner looking at
// several rows knows which one a press ends.
export function useShareRecordName(recordType: RecordType, recordId: string) {
  const t = useT();
  return useEntityName(recordType, recordId).name ?? t("share.thisRecord");
}

// The grantee as the row's own reference names them, so the Revoke label and
// its question say what the reader sees on the row.
function useGranteeLabel(grant: RecordGrant): string {
  const t = useT();
  const { name, reading } = useRosterName(grant.subject_type, grant.subject_id);
  return name ?? missLabel(reading, t, grant.subject_id);
}

/** One share under "Shared with", and the Revoke that ends it. */
export function ShareAccessRow({
  grant,
  recordName,
  mayRevoke,
  onRevoke,
}: Readonly<{
  grant: RecordGrant;
  recordName: string;
  mayRevoke: boolean;
  onRevoke: (subjectName: string) => void;
}>) {
  const t = useT();
  const subjectName = useGranteeLabel(grant);
  const { locale } = useLocale();
  const zone = useViewerZone();
  return (
    <PanelRow className="share-acl-row">
      <div className="share-acl-who">
        <span className="share-acl-name">
          <SubjectKindIcon kind={grant.subject_type} t={t} />
          <EntityRef kind={grant.subject_type} id={grant.subject_id} />
        </span>
        <div className="share-acl-meta">
          <Badge tone={grant.access === "write" ? "accent" : undefined}>
            {accessLabel(grant.access, t)}
          </Badge>
          <span className="t-caption">
            {t("share.grantedBy")}{" "}
            <EntityRef kind="user" id={grant.granted_by} />
          </span>
          {grant.reason && (
            <span className="t-caption share-acl-reason">{grant.reason}</span>
          )}
          {grant.expires_at && (
            <Badge tone="warning">
              {t("share.until", {
                date: formatDateAbbrev(grant.expires_at, locale, zone),
              })}
            </Badge>
          )}
        </div>
      </div>
      {mayRevoke && (
        <Button
          variant="danger"
          aria-label={t("share.revokeFor", {
            name: subjectName,
            record: recordName,
          })}
          onClick={() => onRevoke(subjectName)}
          data-testid="revoke-grant"
        >
          {t("share.revoke")}
        </Button>
      )}
    </PanelRow>
  );
}
