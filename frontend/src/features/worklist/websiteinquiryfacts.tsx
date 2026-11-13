// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Button } from "../../design-system/atoms";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useOpenSource } from "../../lib/opensource";
import { useViewerZone } from "../../lib/viewerzone";
import { inquiryReasonKey } from "../../lib/websiteinquiries";

export function WebsiteInquiryFacts({
  facts,
}: Readonly<{ facts: components["schemas"]["WebsiteInquiryWork"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const open = useOpenSource();
  const source = facts.source_activity_id;
  return (
    <div className="form-stack">
      {facts.source_label && (
        <p>{t("inquiry.sourceValue", { source: facts.source_label })}</p>
      )}
      <p>
        {facts.received_at
          ? t("inquiry.receivedValue", {
              received: formatDateTime(facts.received_at, locale, zone),
            })
          : t("inquiry.receivedUnavailable")}
      </p>
      <p>
        {t("inquiry.requestTypeValue", {
          type: facts.request_type ?? t("inquiry.notSpecified"),
        })}
      </p>
      {facts.record_owner_known && (
        <p>
          {t("inquiry.currentRecordOwner")}:{" "}
          {facts.record_owner_id ? (
            <EntityRef kind="user" id={facts.record_owner_id} />
          ) : (
            t("log.unassigned")
          )}
        </p>
      )}
      <p>
        {t("inquiry.initialHandlingValue", {
          reason: t(inquiryReasonKey(facts.assignment_reason)),
        })}
      </p>
      {source && open && (
        <Button variant="link" onClick={() => open(source)}>
          {t("inquiry.openSubmission")}
        </Button>
      )}
    </div>
  );
}
