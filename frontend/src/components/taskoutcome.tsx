import { useIsMutating } from "@tanstack/react-query";
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
type Link = NonNullable<Activity["links"]>[number];
// An activity needs no record, so "none" is an answer the reader can give.
const UNFILED = "unfiled";
type Filing = Link | typeof UNFILED | null;

export function TaskOutcomeForm({
  task,
  onLogged,
  onBack,
}: Readonly<{ task: Activity; onLogged: () => void; onBack: () => void }>) {
  const t = useT();
  const contact = task.links?.find((link) => link.entity_type === "contact");
  const [filing, setFiling] = useState<Filing>(
    () => contact ?? mostSpecificFirst(task.links ?? [])[0] ?? null,
  );
  const [selecting, setSelecting] = useState(filing === null);
  const record = filing === UNFILED ? null : filing;
  // Back or a new pick unmounts the form, and its remount mints a new
  // idempotency key. So neither is offered while the log is out, or once the
  // activity exists and only its retry is left.
  const [loggedOnly, setLoggedOnly] = useState(false);
  const logging =
    useIsMutating({ mutationKey: ["activity-log", record?.entity_id] }) > 0;
  const held = logging || loggedOnly;
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
      <div className="form-actions">
        <Button variant="ghost" onClick={onBack} disabled={held}>
          {t("tasks.log.back")}
        </Button>
      </div>
      {!contact && (
        <>
          <p>{t("tasks.log.record")}</p>
          {selecting ? (
            <>
              <RecordPicker
                label={t("tasks.log.record")}
                searchTargets={search}
                onPick={(candidate) => {
                  const [kind, id] = candidate.id.split(":");
                  if (kind && isEntityKind(kind) && id) {
                    setFiling({ entity_type: kind, entity_id: id });
                    setSelecting(false);
                  }
                }}
              />
              <div className="form-actions">
                <Button
                  variant="ghost"
                  onClick={() => {
                    setFiling(UNFILED);
                    setSelecting(false);
                  }}
                >
                  {t("tasks.log.noRecord")}
                </Button>
              </div>
            </>
          ) : (
            <div className="form-actions">
              {record && (
                <EntityRef kind={record.entity_type} id={record.entity_id} />
              )}
              <Button
                variant="ghost"
                onClick={() => setSelecting(true)}
                disabled={held}
              >
                {t(record ? "tasks.log.chooseRecord" : "tasks.log.pickRecord")}
              </Button>
            </div>
          )}
        </>
      )}
      {filing !== null && !selecting && (
        <LogActivityForm
          key={task.id}
          askedKind="note"
          entityType={record?.entity_type}
          entityId={record?.entity_id}
          sourceActivityId={task.id}
          taskToComplete={{ id: task.id, version: task.version }}
          inheritSourceLinks={task.links?.some(
            (link) =>
              link.entity_type === record?.entity_type &&
              link.entity_id === record?.entity_id,
          )}
          onLogged={onLogged}
          onLoggedOnly={() => setLoggedOnly(true)}
        />
      )}
    </div>
  );
}
