// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueries, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Disclosure } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { CF_OBJECTS } from "../../lib/customfields.logic";
import { customFieldsQuery } from "../../lib/customfields.query";
import { unwrap } from "../../lib/problem";
import { AuditRail, auditState, type CustomField } from "./customfields.table";

// Every record type's fields by id. An audit entry names its field by id
// alone, and the page's own list holds one record type.
function useFieldsById(enabled: boolean) {
  const catalogs = useQueries({
    queries: CF_OBJECTS.map((object) => customFieldsQuery(object, enabled)),
  });
  const byId = new Map<string, CustomField>(
    catalogs
      .flatMap((catalog) => catalog.data?.data ?? [])
      .map((field) => [field.id, field]),
  );
  return { byId, loading: catalogs.some((catalog) => catalog.isLoading) };
}

/** The changes made to custom fields, behind a row the reader opens. */
export function FieldChangeTrail({
  meUserId,
}: Readonly<{ meUserId?: string }>) {
  const t = useT();
  // The trail is not this screen's: /audit-log asks for `audit_log:read`,
  // while this page opens for anyone holding custom_field:read.
  const readsAuditTrail = useCan("audit_log", "read");
  const [open, setOpen] = useState(false);
  const audit = useQuery({
    queryKey: ["cf-audit"],
    // A denial already known is not asked again: it could only fail, with a
    // retry that never succeeds.
    enabled: readsAuditTrail,
    queryFn: async () => {
      return unwrap(
        await api.GET("/audit-log", {
          params: { query: { entity_type: "custom_field" } },
        }),
      );
    },
  });
  const entries = audit.data?.data ?? [];
  // Read once the reader opens a trail that has rows to name.
  const fields = useFieldsById(open && entries.length > 0);

  return (
    // Withheld, not absent: a section that were not there would read as
    // "nobody has changed a field". Closed by default, as a secondary read.
    <Disclosure summary={t("cf.audit.title")} onToggle={setOpen}>
      <AuditRail
        entries={entries}
        fields={fields.byId}
        state={auditState(
          readsAuditTrail,
          audit.isPending || fields.loading,
          audit.isError,
          entries.length,
        )}
        meUserId={meUserId}
        onRetry={() => void audit.refetch()}
      />
      {/* True for every reader: the recording happens whether or not this
          one may read it back. */}
      <p className="cf-hint t-caption">{t("cf.audit.footer")}</p>
    </Disclosure>
  );
}
