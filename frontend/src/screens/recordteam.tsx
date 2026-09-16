import { X } from "lucide-react";
import { useState } from "react";
import { Button } from "../design-system/atoms";
import { IconAction } from "../design-system/iconaction";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import {
  type AssignmentRecordType,
  type RecordAssignment,
  useArchiveRecordAssignment,
  useRecordAssignments,
} from "./recordassignments.queries";
import { RecordTeamAssign } from "./recordteamassign";

/**
 * Who is responsible for this record, grouped by role.
 *
 * The help text saying this grants no access is not decoration. "Team" reads
 * like a sharing control on every other product a user has met, and somebody
 * who assumes it widens visibility will assign a colleague instead of granting
 * them the record — and then wonder why they still cannot open it.
 *
 * A retired role and a deactivated colleague both keep rendering their label. The
 * row is history by then, and a name that vanished would read as a bug rather
 * than as a responsibility waiting for a successor.
 */
export function RecordTeam({
  recordType,
  recordId,
  readOnly = false,
  bare = false,
}: Readonly<{
  recordType: AssignmentRecordType;
  recordId: string;
  // The caller's own answer to whether this record takes writes at all — the
  // record's per-row `writable`, the archived stamp, the seat ceiling. The
  // panel does not re-derive it: the server refuses an unauthorized write
  // whatever this says, and a second implementation of the gate here is the
  // defect rather than the protection.
  readOnly?: boolean;
  // Drawn as the body of a section something else names, rather than as a
  // panel of its own. The company rail is ONE pane of headed slices, and a
  // titled card standing among them read as a box inside the pane — the one
  // shape the design language refuses. The deal and project pages, columns of
  // cards, keep the panel.
  bare?: boolean;
}>) {
  const t = useT();
  const { data, isPending, isError } = useRecordAssignments(
    recordType,
    recordId,
  );
  const archive = useArchiveRecordAssignment(recordType, recordId);
  // Which assignment the modal is editing: a row to move, "new" to name one,
  // or null for closed. One piece of state rather than an open flag beside a
  // selected row, so the two cannot disagree about what the modal is for.
  const [editing, setEditing] = useState<RecordAssignment | "new" | null>(null);
  const rows = data ?? [];
  // A failed read is not an empty record. Offering "assign somebody" over a
  // list that could not be fetched invites a duplicate of a responsibility
  // that may already be there.
  const canWrite = !readOnly && !isPending && !isError;
  const assign = canWrite ? (
    <Button small={bare} variant="ghost" onClick={() => setEditing("new")}>
      {t("assignments.add")}
    </Button>
  ) : undefined;
  const modal = (
    <RecordTeamAssign
      open={editing !== null}
      onClose={() => setEditing(null)}
      recordType={recordType}
      recordId={recordId}
      existing={editing === "new" ? undefined : (editing ?? undefined)}
    />
  );
  const body = (
    <>
      <PanelBody>
        {/* A refused end is the one failure here a reader must not have to
            infer. The button re-enables when the request settles either way,
            so without this a responsibility that is still standing looks
            exactly like one that ended — and the next thing the reader does,
            they do believing it is gone. */}
        {archive.isError && (
          <p className="t-caption" role="alert">
            {problemMessageOf(archive.error, t)}
          </p>
        )}
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
          <>
            {/* Said only once there is a row to misread as access: an empty
                record has no role for a reader to mistake for a door, so the
                detail below already covers it and this line would just repeat
                it. */}
            <p className="t-caption">{t("assignments.noAccessNote")}</p>
            <ul className="firmo assignments">
              {rows.map((row) => (
                <AssignmentRow
                  key={row.id}
                  row={row}
                  canWrite={canWrite}
                  busy={archive.isPending}
                  onChange={() => setEditing(row)}
                  onRemove={() => archive.mutate(row.id)}
                />
              ))}
            </ul>
          </>
        )}
        {/* Under the rows in the bare shape too, where the panel's own verb
            band would have put it: on the body's own left edge, the same x
            the Active deals section's own verb sits on. */}
        {bare && assign && <div className="card-actions">{assign}</div>}
      </PanelBody>
      {modal}
    </>
  );
  if (bare) {
    return body;
  }
  return (
    <Panel title={t("assignments.title")} actions={assign}>
      {body}
    </Panel>
  );
}

/**
 * One responsibility. The role is the eyebrow and the colleague or team is the
 * value, because a reader scans this list asking "who has this job", not "what
 * jobs does this colleague have".
 */
function AssignmentRow({
  row,
  canWrite,
  busy,
  onChange,
  onRemove,
}: Readonly<{
  row: RecordAssignment;
  canWrite: boolean;
  busy: boolean;
  onChange: () => void;
  onRemove: () => void;
}>) {
  const t = useT();
  return (
    <li>
      {/* The role over the name it belongs to. Wrapped so a long team name
          wraps within the pair rather than pushing the verbs below off the
          row's own column. */}
      <span className="assignrow-pair">
        <span className="t-eyebrow">
          {row.role_label ?? row.role_key}
          {row.role_active === false && ` ${t("assignments.roleRetired")}`}
        </span>
        <span>
          {row.subject_name}
          {/* A team and a colleague read identically otherwise, and which one
              holds a responsibility changes who to ask. */}
          {row.subject_kind === "team" && ` ${t("assignments.teamSuffix")}`}
          {row.subject_inactive && ` ${t("assignments.subjectInactive")}`}
        </span>
      </span>
      {canWrite && (
        <span className="assignrow-verbs">
          {/* Named with the row's subject, because a list of responsibilities
              draws one of these per row and "Change" alone tells a reader on a
              screen reader nothing about which one they are on. Small: the
              rail is narrow enough that a full-size pair pushed the role and
              name onto two wrapped lines each. */}
          <Button
            small
            variant="ghost"
            onClick={onChange}
            disabled={busy}
            aria-label={t("assignments.changeOne", { who: row.subject_name })}
          >
            {t("assignments.change")}
          </Button>
          {/* Squared, like the fact rows' own removal verb: the X glyph is
              already the word "remove" to a reader who has met it once, and
              the row still owes the same named refusal a full-word button
              would. */}
          <IconAction
            label={t("assignments.removeOne", { who: row.subject_name })}
            icon={<X aria-hidden />}
            small
            disabled={busy}
            onClick={onRemove}
          />
        </span>
      )}
    </li>
  );
}
