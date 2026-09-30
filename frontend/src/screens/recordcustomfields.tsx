// Which of the two custom-field surfaces is for what: customfields.card.tsx.
import type { ReactNode } from "react";
import { useCanWriteRecord } from "../app/capability";
import { Button } from "../design-system/atoms";
import { OffsiteLink } from "../design-system/offsitelink";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import {
  type CustomField,
  customFieldHref,
  useCustomFieldDisplay,
  useObjectCustomFields,
} from "./customfields.form";
import { saveRecordEdit } from "./recordedit";
import { type FieldRecord, RecordFields } from "./recordfields";

type Kind = "company" | "contact" | "deal" | "lead";
export function RecordCustomFields({
  kind,
  record,
}: Readonly<{
  kind: Kind;
  record: FieldRecord & { writable?: boolean; archived_at?: string | null };
}>) {
  const t = useT();
  const cf = useObjectCustomFields(kind);
  const read = useCustomFieldDisplay();
  const canEdit = useCanWriteRecord(kind, record) && !record.archived_at;
  if (cf.loading || cf.failed)
    return (
      <Panel title={t("cf.formSection")}>
        <PanelBody>
          <p role={cf.failed ? "alert" : "status"}>
            {t(cf.failed ? "record.fieldsFailed" : "record.fieldsLoading")}
          </p>
          {cf.failed && (
            <Button onClick={() => cf.retry()}>
              {t("record.fieldsRetry")}
            </Button>
          )}
        </PanelBody>
      </Panel>
    );
  if (!cf.fields.length) return null;
  const { displayValues, renderValues } = readings(cf.fields, record, read);
  return (
    <RecordFields
      displayValues={displayValues}
      renderValues={renderValues}
      title={t("cf.formSection")}
      kind={kind}
      fields={cf.formFields}
      maskedFields={
        Array.isArray(record.masked_fields)
          ? record.masked_fields.filter(
              (key): key is string => typeof key === "string",
            )
          : []
      }
      readOnlyFields={Object.fromEntries(
        (Array.isArray(record.masked_fields) ? record.masked_fields : [])
          .filter((key): key is string => typeof key === "string")
          .map((key) => [key, t("record.notShown")]),
      )}
      record={record}
      canEdit={canEdit}
      save={async (values, _rows, opened) => {
        return saveRecordEdit(kind, opened, cf.toPatch(values, opened));
      }}
    />
  );
}

function readings(
  fields: CustomField[],
  record: FieldRecord,
  read: ReturnType<typeof useCustomFieldDisplay>,
) {
  const displayValues: Record<string, string> = {};
  const renderValues: Record<string, ReactNode> = {};
  for (const field of fields) {
    const display = read(field, record[field.column_name]);
    if (!display) continue;
    displayValues[field.column_name] = display;
    const href = customFieldHref(display);
    // Not a picklist: InlineChoice's trigger is its value and has no verb mode.
    if (href && field.type === "text")
      renderValues[field.column_name] = (
        <OffsiteLink href={href}>{display}</OffsiteLink>
      );
  }
  return { displayValues, renderValues };
}
