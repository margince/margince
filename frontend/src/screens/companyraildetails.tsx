// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { Hash, MapPin } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useCanWriteRecord } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { useUnsavedGuard } from "../app/unsaved";
import { EvidenceMark } from "../design-system/evidencemark";
import { FieldGrid, FieldRow } from "../design-system/fieldgrid";
import { InlineText } from "../design-system/inlinechoice";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import { CompanyDetails } from "./companydetails";
import { useCompanyReadOnlyReason } from "./companyheader";
import { VatMark } from "./companyvatmark";
import { derivedSource } from "./evidencesource";
import { profileFieldsKey, useCompanyProfileFields } from "./evidenceverdict";

// The rail's own Details grid (companyrail.tsx's DetailsGrid), split into
// this file so the rail file stays under the 500-line ceiling: one panel
// section (Details) and its field rows is a natural seam, not an arbitrary
// cut.

type Company = components["schemas"]["Company"];
type ProfileField = components["schemas"]["CompanyProfileField"];
// The path parameter's own closed vocabulary, taken from the generated contract
// rather than respelled: a field name this endpoint does not accept is then a
// compile error here rather than a 422 the reader meets.
type ProfileFieldKey = components["parameters"]["ProfileFieldKey"];
// Not `keyof Address`: the wire type carries a `[key: string]: unknown` catch-all
// alongside its six named parts (schema.d.ts's own escape hatch for a future
// field), which collapses `keyof` down to bare `string` and loses every part's
// actual value type. The six literals are the only ones this row ever writes.
export function DetailsGrid({ company }: Readonly<{ company?: Company }>) {
  if (!company) {
    return null;
  }
  // Split into its own component (rather than returning early above and
  // calling the hooks below unconditionally) so every hook in this file runs
  // on every render of THIS component and stays absent entirely on the
  // no-company one — an early return between hook calls fails the Rules
  // of Hooks the moment `company` flips between defined and not, which a
  // 360 read that answers slower than the shell mount does routinely.
  return <DetailsGridBody company={company} />;
}

// The two legal-identity fields that live only in the evidence sidecar: the
// VAT/tax identifier and the address the company is REGISTERED at.
//
// Neither has a column on `company`, so neither can ride the rows above:
// they are written through the profile-field correction path instead. That
// difference is invisible to a reader and should stay so — a rep stating the
// company's VAT number is doing the same thing as stating its legal name, and
// the two rows sit together because they are the same kind of fact.
//
// `registered_address` is NOT the postal address in the disclosure below. That
// one is six columns describing where the company OPERATES; this one is the
// single line a register prints. Collapsing them was never intended, so the
// labels have to keep them apart.
const SIDECAR_FIELDS = [
  {
    field: "register_vat",
    labelKey: "co.profileField.register_vat",
    icon: <Hash />,
    placeholderKey: "field.addRegisterVat",
  },
  {
    field: "registered_address",
    labelKey: "co.profileField.registered_address",
    icon: <MapPin />,
    placeholderKey: "field.addRegisteredAddress",
  },
] as const satisfies readonly {
  field: ProfileFieldKey;
  labelKey: MessageKey;
  icon: ReactNode;
  placeholderKey: MessageKey;
}[];

// One sidecar field's row. The value comes from the profile-fields read rather
// than from `company`, which carries no sidecar claim.
// Exported because the Profile tab writes the SAME sidecar fields through the
// same PATCH — one writer for a profile-field value, mounted twice, so the rail
// and the tab can never disagree about what they last wrote or which
// precondition they sent. `multiline` is the tab's case: the narrative fields
// (what they sell, who they sell to) are paragraphs, and a paragraph in a
// single-line input is a field a reader cannot read back while typing it.
export function SidecarFieldRow({
  companyId,
  fields,
  fieldsLoaded,
  field,
  labelKey,
  label: labelText,
  icon,
  placeholderKey,
  canEdit,
  readOnlyReason,
  multiline,
  guardDetails,
}: Readonly<{
  companyId: string;
  fields: readonly ProfileField[];
  // Whether `fields` is an ANSWER or merely the empty list a pending or failed
  // read stands in with. The two are not the same claim: an answered read
  // saying a field has no row means the write creates one, while an unanswered
  // read saying the same thing means nobody knows — and sending no If-Match on
  // that guess overwrites a value the reader never saw. Editing waits for the
  // answer.
  fieldsLoaded: boolean;
  field: ProfileFieldKey;
  // Two ways to name the row, because the two callers know the name
  // differently: the rail states its two fields literally, and the Profile tab
  // reads eleven of them out of the one vocabulary map that the backend gate
  // holds. Exactly one is required, which the callers' own types enforce.
  labelKey?: MessageKey;
  label?: string;
  // The kind-of-fact glyph, when the caller knows the kind. The Profile tab
  // draws eleven fields out of one vocabulary map and names none of them
  // individually, so it passes none and its rows read as they always have.
  icon?: ReactNode;
  placeholderKey: MessageKey;
  canEdit: boolean;
  readOnlyReason: string | undefined;
  multiline?: boolean;
  guardDetails?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [dirty, setDirty] = useState(false);
  useUnsavedGuard(dirty, guardDetails ? "details" : undefined);
  const label = labelText ?? (labelKey ? t(labelKey) : field);
  const current = fields.find((one) => one.field === field);
  const save = async (next: string) => {
    const { error } = await api.PATCH(
      "/companies/{id}/profile-fields/{field}",
      {
        params: {
          path: { id: companyId, field },
          // A field nobody has stated yet has no row and so no version to pin:
          // the write CREATES it, and there is no earlier state to lose. Once
          // one exists the precondition is what stops two contacts correcting the
          // same claim and the second silently replacing the first.
          ...(current ? ifMatch(requireVersion(current.version)) : {}),
        },
        body: { value: next.trim() },
      },
    );
    if (error) {
      throwProblem(error);
    }
    // The record read and the profile-fields read both now describe the write
    // that just landed, and the 360 summarises it.
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: profileFieldsKey(companyId) }),
      queryClient.invalidateQueries({ queryKey: ["company", companyId] }),
      queryClient.invalidateQueries({ queryKey: ["company360", companyId] }),
    ]);
  };
  // A value a machine read keeps its dotted underline and its receipt. The mark
  // wraps the RESTING value only — while the reader is typing, the claim under
  // edit is theirs, and an underline saying "a crawl found this" over their own
  // draft would be false. Absent for a value a human typed: derivedSource
  // returns nothing for a human row, and a mark with nothing behind it teaches
  // the reader to stop opening them.
  const source = current
    ? derivedSource(current, locale, recordZone)
    : undefined;
  return (
    <FieldRow label={label} icon={icon}>
      {/* Editing and provenance are two controls, not one: EvidenceMark makes
          its value the button that opens the receipt, and InlineText makes it
          the button that starts an edit. One element cannot be both, so the
          value stays editable and the receipt sits beside it — the same shape
          the VAT verdict already uses on this grid. A human-typed value has no
          receipt and shows no mark. */}
      <InlineText
        label={label}
        value={current?.value ?? ""}
        placeholder={t(placeholderKey)}
        multiline={multiline}
        canEdit={canEdit && fieldsLoaded}
        readOnlyReason={readOnlyReason}
        onEditingChange={setEditing}
        onDirtyChange={setDirty}
        onSave={save}
      />
      {/* Only while the stored value is what is on screen. Mid-edit the text
          belongs to the reader, and a receipt saying "a crawl read this" beside
          their own draft attributes their words to a machine — indefinitely, if
          the save then fails and the draft stays. */}
      {source && current && !editing && (
        <EvidenceMark value={t("evidence.mark")} source={source} />
      )}
      {/* The register's verdict beside the number it answers for. Only once a
          number exists: a mark on an empty field would say the register had
          declined to recognise something nobody has stated. */}
      {field === "register_vat" && current?.value && (
        <VatMark
          companyId={companyId}
          stated={current.value}
          canAsk={canEdit}
        />
      )}
    </FieldRow>
  );
}

export function CompanyProfileDetails({
  company,
}: Readonly<{ company: Company }>) {
  const canUpdate = useCanWriteRecord("company", company);
  const readOnlyReason = useCompanyReadOnlyReason(company);
  const query = useCompanyProfileFields(company.id);
  return (
    <FieldGrid icons>
      {SIDECAR_FIELDS.map((sidecar) => (
        <SidecarFieldRow
          key={sidecar.field}
          guardDetails
          companyId={company.id}
          fields={query.data ?? []}
          fieldsLoaded={query.isSuccess}
          canEdit={canUpdate && !readOnlyReason}
          readOnlyReason={readOnlyReason}
          {...sidecar}
        />
      ))}
    </FieldGrid>
  );
}
function DetailsGridBody({ company }: Readonly<{ company: Company }>) {
  return (
    <>
      <CompanyDetails company={company} />
      <CompanyProfileDetails company={company} />
    </>
  );
}
