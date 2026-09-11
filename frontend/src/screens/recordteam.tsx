import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import {
  type AssignmentRecordType,
  type RecordAssignment,
  useRecordAssignments,
} from "./recordassignments.queries";

/**
 * Who is responsible for this record, grouped by role.
 *
 * The help text saying this grants no access is not decoration. "Team" reads
 * like a sharing control on every other product a user has met, and somebody
 * who assumes it widens visibility will assign a colleague instead of granting
 * them the record — and then wonder why they still cannot open it.
 *
 * A retired role and a deactivated person both keep rendering their label. The
 * row is history by then, and a name that vanished would read as a bug rather
 * than as a responsibility waiting for a successor.
 */
export function RecordTeam({
  recordType,
  recordId,
}: Readonly<{ recordType: AssignmentRecordType; recordId: string }>) {
  const t = useT();
  const { data, isPending, isError } = useRecordAssignments(
    recordType,
    recordId,
  );
  const rows = data ?? [];
  return (
    <Panel title={t("assignments.title")}>
      <PanelBody>
        <p className="t-caption mute">{t("assignments.noAccessNote")}</p>
        {isPending || isError || rows.length === 0 ? (
          <SurfaceState
            state={isPending ? "loading" : isError ? "failed" : "empty"}
            emptyLabel={t("assignments.empty")}
            emptyDetail={t("assignments.emptyDetail")}
            loadingLabel={t("assignments.title")}
          >
            {null}
          </SurfaceState>
        ) : (
          <ul className="firmo">
            {rows.map((row) => (
              <AssignmentRow key={row.id} row={row} />
            ))}
          </ul>
        )}
      </PanelBody>
    </Panel>
  );
}

/**
 * One responsibility. The role is the eyebrow and the person or team is the
 * value, because a reader scans this list asking "who has this job", not "what
 * jobs does this person have".
 */
function AssignmentRow({ row }: Readonly<{ row: RecordAssignment }>) {
  const t = useT();
  return (
    <li>
      <span className="t-eyebrow">
        {row.role_label ?? row.role_key}
        {row.role_active === false && ` ${t("assignments.roleRetired")}`}
      </span>
      <span>
        {row.subject_name}
        {/* A team and a person read identically otherwise, and which one holds
            a responsibility changes who to ask. */}
        {row.subject_kind === "team" && ` ${t("assignments.teamSuffix")}`}
        {row.subject_inactive && ` ${t("assignments.subjectInactive")}`}
      </span>
    </li>
  );
}
