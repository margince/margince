// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Popover } from "../../design-system/popover";
import { provenanceLabel } from "../../design-system/provenance";
import { provenanceOf } from "../../design-system/provenanceof";
import type { DetailRow } from "../../design-system/recorddetails";
import { formatDateAbbrev } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useMemberName } from "../../lib/membernames";
import { useRecordZone } from "../../lib/recordzone";
import { useViewerId } from "../../lib/session";
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
// row names who, so the two read the same `visibility` and `owner_id` apart.
function VisibleTo({ company }: Readonly<{ company: Company }>) {
  const t = useT();
  const viewerId = useViewerId();
  const owner = company.owner_id ?? undefined;
  const yours = viewerId !== undefined && owner === viewerId;
  const closed = company.visibility === "owner";
  const name = useMemberName(closed && !yours ? owner : null);
  if (!closed) {
    return <>{t("co.aside.visibleEveryone")}</>;
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

/** The register's facts about the account and how its own row was written. */
export function useRegistrationRows(company: Company, edit: Edit): DetailRow[] {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
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
  const provenance = provenanceOf(
    company.captured_by,
    viewerId,
    company.author,
    company.source,
    company.imported_from,
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
