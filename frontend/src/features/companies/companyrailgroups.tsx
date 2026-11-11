// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Popover } from "../../design-system/popover";
import { provenanceLabel } from "../../design-system/provenance";
import { provenanceOf, withCreator } from "../../design-system/provenanceof";
import type { DetailRow } from "../../design-system/recorddetails";
import { formatDateAbbrev, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useMemberName } from "../../lib/membernames";
import { unwrap } from "../../lib/problem";
import { useViewerId } from "../../lib/session";
import { useViewerZone } from "../../lib/viewerzone";
import { CompanyOwnerControl } from "./companyheader";
import type { Edit } from "./companyrail";
import { SidecarFieldValue } from "./companyraildetails";
import { useCompanyProfileFields } from "./evidenceverdict";

type Company = components["schemas"]["Company"];

/** Who holds the account and who may read it. */
export function useOwnershipRows(company: Company): DetailRow[] {
  const t = useT();
  return [
    {
      key: "owner",
      term: t("co.pulse.owner"),
      // The control, not a reading: on an unowned account it is the claim door.
      value: <CompanyOwnerControl company={company} hideLabel />,
    },
    {
      key: "visible",
      term: t("record.details.visibleTo"),
      value: <VisibleTo company={company} />,
    },
  ];
}

// The audience the head's Shared pill switches, as the row's one word. Not
// RecordAccess's sentence: that one explains the setting in a popover, this
// row names who. On a closed record the shares widen that audience, so they
// count here too. A refused grants read leaves the owner alone, not a guess.
function VisibleTo({ company }: Readonly<{ company: Company }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const viewerId = useViewerId();
  const owner = company.owner_id ?? undefined;
  const yours = viewerId !== undefined && owner === viewerId;
  const closed = company.visibility === "owner";
  const name = useMemberName(closed && !yours ? owner : null);
  const shares = useQuery({
    // Under the share dialog's key, so a new share refreshes it. The last part
    // keeps this count apart from the dialog's list of grants.
    queryKey: ["record-grants", "company", company.id, "count"],
    queryFn: () => fetchShareCount(company.id),
    enabled: closed,
    retry: false,
  });
  if (!closed) {
    return <>{t("co.aside.visibleEveryone")}</>;
  }
  const shared = shares.data?.count ?? 0;
  if (shared > 0) {
    const formatted = formatNumber(shared, locale);
    const count = shares.data?.more ? `${formatted}+` : formatted;
    if (yours) {
      return <>{plural("co.aside.visibleYouAndShared", shared, { count })}</>;
    }
    return (
      <>
        {name.data
          ? plural("co.aside.visibleOwnerAndShared", shared, {
              owner: name.data,
              count,
            })
          : plural("co.aside.visibleTheOwnerAndShared", shared, { count })}
      </>
    );
  }
  if (yours) {
    return <>{t("co.aside.visibleOnlyYou")}</>;
  }
  return (
    <>
      {name.data
        ? t("co.aside.visibleOnly", { owner: name.data })
        : t("co.aside.visibleOnlyOwner")}
    </>
  );
}

// One page of grants is the count. A page the server cut says so, and the row
// shows "100+" instead of a smaller number that reads as the whole audience.
async function fetchShareCount(
  companyId: string,
): Promise<{ count: number; more: boolean }> {
  const data = unwrap(
    await api.GET("/record-grants", {
      params: {
        query: { record_type: "company", record_id: companyId, limit: 100 },
      },
    }),
  );
  return { count: data.data.length, more: data.page.has_more };
}

/** The register's facts about the account and how its own row was written. */
export function useRegistrationRows(company: Company, edit: Edit): DetailRow[] {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const profile = useCompanyProfileFields(company.id);
  return [
    {
      key: "vat",
      term: t("co.aside.vatId"),
      value: (
        <SidecarFieldValue
          companyId={company.id}
          anchor={company.is_anchor === true}
          fields={profile.data ?? []}
          fieldsLoaded={profile.isSuccess}
          field="register_vat"
          label={t("co.aside.vatId")}
          placeholderKey="co.aside.add"
          canEdit={edit.canEdit}
          readOnlyReason={edit.reason}
        />
      ),
    },
    {
      key: "source",
      term: t("history.field.source"),
      value: <CompanySource company={company} />,
    },
    {
      key: "created",
      term: t("list.created"),
      value: formatDateAbbrev(company.created_at, locale, zone),
    },
  ];
}

// The kind of source as plain words, the way the design reads the row; which
// one, behind it.
function CompanySource({ company }: Readonly<{ company: Company }>) {
  const t = useT();
  const viewerId = useViewerId();
  const provenance = withCreator(
    provenanceOf(
      company.captured_by,
      viewerId,
      company.author,
      company.source,
      company.imported_from,
    ),
    company.created_by,
  );
  const authorName = useMemberName(
    provenance.kind === "human" ? provenance.userId : undefined,
  );
  return (
    <Popover
      label={provenanceLabel(provenance, t, () => authorName.data ?? undefined)}
    >
      <p className="t-body">{company.source || t("trust.sourceUnknown")}</p>
    </Popover>
  );
}
