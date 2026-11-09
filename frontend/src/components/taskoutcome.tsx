import { useCallback, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { RecordPicker } from "../design-system/recordpicker";
import { useT } from "../i18n";
import { ENTITY_KINDS, isEntityKind } from "../lib/entity";
import { unwrap } from "../lib/problem";
import { EntityRef } from "./entityref";
import { LogActivityForm } from "./logactivity";
import { mostSpecificFirst } from "./recordlinks";

type Activity = components["schemas"]["Activity"];
export function TaskOutcomeForm({
  task,
  onLogged,
}: Readonly<{ task: Activity; onLogged: () => void }>) {
  const t = useT();
  const contact = task.links?.find((link) => link.entity_type === "contact");
  const [record, setRecord] = useState(
    () => contact ?? mostSpecificFirst(task.links ?? [])[0] ?? null,
  );
  const [selecting, setSelecting] = useState(record === null);
  const search = useCallback(async (q: string) => {
    const page = unwrap(
      await api.GET("/search", {
        params: { query: { q, types: [...ENTITY_KINDS], limit: 20 } },
      }),
    );
    return page.data.flatMap((hit) =>
      isEntityKind(hit.type) && hit.title
        ? [{ id: `${hit.type}:${hit.id}`, name: hit.title }]
        : [],
    );
  }, []);
  return (
    <div className="form-stack">
      {!contact && (
        <>
          <p>{t("tasks.log.record")}</p>
          {selecting ? (
            <RecordPicker
              label={t("tasks.log.record")}
              searchTargets={search}
              onPick={(candidate) => {
                const [kind, id] = candidate.id.split(":");
                if (kind && isEntityKind(kind) && id) {
                  setRecord({
                    entity_type: kind,
                    entity_id: id,
                  });
                  setSelecting(false);
                }
              }}
            />
          ) : (
            record && (
              <div className="form-actions">
                <EntityRef kind={record.entity_type} id={record.entity_id} />
                <Button variant="ghost" onClick={() => setSelecting(true)}>
                  {t("tasks.log.chooseRecord")}
                </Button>
              </div>
            )
          )}
        </>
      )}
      {record && !selecting && (
        <LogActivityForm
          askedKind="note"
          entityType={record.entity_type}
          entityId={record.entity_id}
          sourceActivityId={task.id}
          taskToComplete={{ id: task.id, version: task.version }}
          inheritSourceLinks={task.links?.some(
            (link) =>
              link.entity_type === record.entity_type &&
              link.entity_id === record.entity_id,
          )}
          onLogged={onLogged}
        />
      )}
    </div>
  );
}
