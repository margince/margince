// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { EntityRef, useEntityName } from "../../components/entityref";
import {
  accessLabel,
  type RecordGrant,
  type RecordType,
  type RosterSubject,
  SubjectKindIcon,
} from "../../components/share.parts";
import { Badge, Button } from "../../design-system/atoms";
import { PanelRow } from "../../design-system/panel";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";

// Revoke and the grant toast say whose access to what, so an owner looking at
// several rows knows which one a press ends.
export function useShareNames(
  recordType: RecordType,
  recordId: string,
  roster: readonly RosterSubject[],
) {
  const t = useT();
  const recordName =
    useEntityName(recordType, recordId).name ?? t("share.thisRecord");
  const subjectName = (grantRow: RecordGrant) =>
    roster.find(
      (entry) =>
        entry.kind === grantRow.subject_type &&
        entry.id === grantRow.subject_id,
    )?.name ?? t("share.unnamedSubject");
  return { recordName, subjectName };
}

/** One share under "Shared with", and the Revoke that ends it. */
export function ShareAccessRow({
  grant,
  subjectName,
  recordName,
  mayRevoke,
  onRevoke,
}: Readonly<{
  grant: RecordGrant;
  subjectName: string;
  recordName: string;
  mayRevoke: boolean;
  onRevoke: () => void;
}>) {
  const t = useT();
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
          {grant.reason && <span className="t-caption">{grant.reason}</span>}
          {grant.expires_at && (
            <Badge tone="warning">
              {formatDate(grant.expires_at, locale, zone)}
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
          onClick={onRevoke}
          data-testid="revoke-grant"
        >
          {t("share.revoke")}
        </Button>
      )}
    </PanelRow>
  );
}
