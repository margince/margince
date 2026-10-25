// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { RELATIONSHIP_TYPE_LABELS } from "../../components/companylookups";
import { EntityRef } from "../../components/entityref";
import { FieldGrid, FieldRow } from "../../design-system/fieldgrid";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import {
  companyTypeLabel,
  useCompanyTypeDirectory,
} from "../../lib/companytypes";
import { type Edit, useRowBuilders } from "./companyrail";
import { SidecarFieldValue } from "./companyraildetails";
import { CompanyTagsSection } from "./companyrailtags";

type Company = components["schemas"]["Company"];
type ProfileField = components["schemas"]["CompanyProfileField"];

/**
 * The account's filed fields that the record's details column does not draw:
 * legal name, relationship types, parent company, LinkedIn, registered
 * address and tags. They sit here, every one visible and editable in place.
 * The details column then holds only what the design shows, and no value on
 * the record becomes unreachable.
 */
export function CompanyProfileDetails({
  company,
  edit,
  fields,
  fieldsLoaded,
}: Readonly<{
  company: Company;
  edit: Edit;
  // The page's one profile-fields read, so this panel and the narrative
  // below it write against the same answer.
  fields: readonly ProfileField[];
  fieldsLoaded: boolean;
}>) {
  const t = useT();
  const { text, compound } = useRowBuilders(company, edit);
  const extensionTypes = useCompanyTypeDirectory().data?.data ?? [];
  // The in-place edit writes core's list only, so an extension's types are
  // shown beside it and stay as they are.
  const typeLabels = [
    ...(company.relationship_types ?? []).map((type) =>
      t(RELATIONSHIP_TYPE_LABELS[type]),
    ),
    ...(company.extension_relationship_types ?? []).map((type) =>
      companyTypeLabel(type, extensionTypes),
    ),
  ];
  const rows = [
    text("legal_name", "create.legalName"),
    compound(
      "relationship_types",
      t("company.relationshipTypes"),
      typeLabels.length > 0 ? typeLabels.join(", ") : undefined,
    ),
    compound(
      "parent_company_id",
      t("history.field.parent_company_id"),
      company.parent_company_id ? (
        <EntityRef kind="company" id={company.parent_company_id} />
      ) : undefined,
    ),
    text("linkedin_url", "create.linkedinUrl"),
    {
      key: "registered_address",
      term: t("co.profileField.registered_address"),
      value: (
        <SidecarFieldValue
          companyId={company.id}
          anchor={company.is_anchor === true}
          fields={fields}
          fieldsLoaded={fieldsLoaded}
          field="registered_address"
          labelKey="co.profileField.registered_address"
          placeholderKey="field.addRegisteredAddress"
          canEdit={edit.canEdit}
          readOnlyReason={edit.reason}
        />
      ),
    },
    {
      key: "tags",
      term: t("tags.panelTitle"),
      value: (
        <CompanyTagsSection company={company} companyId={company.id} bare />
      ),
    },
  ];
  return (
    <Panel title={t("co.details.title")}>
      <PanelBody>
        <FieldGrid>
          {rows.map((row) => (
            <FieldRow key={row.key} label={row.term}>
              {row.value}
            </FieldRow>
          ))}
        </FieldGrid>
      </PanelBody>
    </Panel>
  );
}
