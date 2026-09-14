import { useCanWriteRecord } from "../app/capability";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { useSorMode } from "./common";
import { useObjectCustomFields } from "./customfields.form";
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
  const canEdit = useCanWriteRecord(kind, record) && !record.archived_at;
  const overlay = useSorMode() === "overlay";
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
  return (
    <RecordFields
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
      notice={overlay ? t("overlay.partialWriteBack") : undefined}
      save={async (values, _rows, opened) => {
        return saveRecordEdit(kind, opened, cf.toPatch(values, opened));
      }}
    />
  );
}
