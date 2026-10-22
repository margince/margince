// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { useUnsavedGuard } from "../../components/unsaved";
import { EvidenceMark } from "../../design-system/evidencemark";
import { FieldRow } from "../../design-system/fieldgrid";
import { InlineText } from "../../design-system/inlinetext";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";
import { invalidateCompany } from "../../lib/recordwritekeys";
import { useRecordZone } from "../../lib/recordzone";
import { VatMark } from "./companyvatmark";
import { derivedSource } from "./evidencesource";
import { profileFieldsKey } from "./evidenceverdict";

type ProfileField = components["schemas"]["CompanyProfileField"];
// The path parameter's own closed vocabulary, taken from the generated contract
// rather than respelled: a field name this endpoint does not accept is then a
// compile error here rather than a 422 the reader meets.
type ProfileFieldKey = components["parameters"]["ProfileFieldKey"];

// One sidecar field: a value that has no column on `company` and is written
// through the profile-field path. The details' Registration rows and the
// Profile tab write the same fields through this one writer. So the two never
// disagree about what they last wrote or which precondition they sent.
export function SidecarFieldRow(props: SidecarFieldProps) {
  const t = useT();
  const label =
    props.label ?? (props.labelKey ? t(props.labelKey) : props.field);
  return (
    <FieldRow label={label} icon={props.icon}>
      <SidecarFieldValue {...props} />
    </FieldRow>
  );
}

type SidecarFieldProps = Readonly<{
  companyId: string;
  // The record's `is_anchor`: its fields are also the settings company profile.
  anchor: boolean;
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
}>;

/** One sidecar field's value and its marks, for a row its caller draws. */
export function SidecarFieldValue({
  companyId,
  anchor,
  fields,
  fieldsLoaded,
  field,
  labelKey,
  label: labelText,
  placeholderKey,
  canEdit,
  readOnlyReason,
  multiline,
  guardDetails,
}: SidecarFieldProps) {
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
    unwrap(
      await api.PATCH("/companies/{id}/profile-fields/{field}", {
        params: {
          path: { id: companyId, field },
          // A field nobody has stated yet has no row and so no version to pin:
          // the write CREATES it, and there is no earlier state to lose. Once
          // one exists the precondition is what stops two contacts correcting the
          // same claim and the second silently replacing the first.
          ...(current ? ifMatch(requireVersion(current.version)) : {}),
        },
        body: { value: next.trim() },
      }),
    );
    // The record read and the profile-fields read both now describe the write
    // that just landed, and the 360 summarises it.
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: profileFieldsKey(companyId) }),
      invalidateCompany(queryClient, { id: companyId, is_anchor: anchor }),
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
    <>
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
        <EvidenceMark
          value={t("evidence.mark")}
          subject={current.value}
          source={source}
        />
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
    </>
  );
}
