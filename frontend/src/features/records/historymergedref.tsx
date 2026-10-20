import { EntityRef, useEntityName } from "../../components/entityref";
import { useT } from "../../i18n";
import type { EntityKind } from "../../lib/entity";

// The record a merge folded in, or folded this one into, by its name.
// The name is read by id like any reference, so the record read decides
// whether this reader may have it. A record they may not read is "another
// record": the entry still says a merge happened without naming it.
const MERGE_REFERENCE_FIELDS: ReadonlySet<string> = new Set([
  "merged_from_id",
  "merged_into_id",
]);

export function isMergeReference(field: string): boolean {
  return MERGE_REFERENCE_FIELDS.has(field);
}

export function MergedRecordName({
  kind,
  id,
}: Readonly<{ kind: EntityKind; id: string }>) {
  const t = useT();
  const { name, reading } = useEntityName(kind, id);
  if (name !== null) {
    return <EntityRef kind={kind} id={id} name={name} />;
  }
  return (
    <span>
      {reading === "pending"
        ? t("common.loading")
        : t("history.mergedRecordUnnamed")}
    </span>
  );
}
