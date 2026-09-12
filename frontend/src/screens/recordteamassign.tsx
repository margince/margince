import { useCallback, useEffect, useId, useMemo, useState } from "react";
import { api } from "../api/client";
import { Button, Modal, SegmentedControl } from "../design-system/atoms";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import {
  type AssignmentRecordType,
  type AssignmentSubjectKind,
  assignableRoles,
  type RecordAssignment,
  useCreateRecordAssignment,
  useRecordRoles,
  useUpdateRecordAssignment,
} from "./recordassignments.queries";

const SUBJECT_KINDS: readonly AssignmentSubjectKind[] = ["user", "team"];

/**
 * Naming who is responsible for a record, or moving that responsibility.
 *
 * One modal for both verbs. Adding and reassigning ask the identical three
 * questions — colleague or team, which one, under which role — and the server
 * takes the same shape for each; two modals would be two places for that shape
 * to drift.
 *
 * The role list is filtered to what the server would actually accept for THIS
 * record type and THIS kind of assignee (`assignableRoles`), so a reader cannot
 * pick a combination whose only feedback is a refused save. Changing the
 * colleague/team choice can therefore invalidate a role already picked, which
 * is why the role clears with it rather than being carried into a list that no
 * longer offers it.
 */
export function RecordTeamAssign({
  open,
  onClose,
  recordType,
  recordId,
  existing,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  recordType: AssignmentRecordType;
  recordId: string;
  // The assignment being moved, or undefined when naming a new one. Its
  // subject and role seed the form so a reader changing one of the two does
  // not have to restate the other.
  existing?: RecordAssignment;
}>) {
  const t = useT();
  const headingId = useId();
  const [subjectKind, setSubjectKind] = useState<AssignmentSubjectKind>("user");
  const [subject, setSubject] = useState<RecordPickerCandidate | null>(null);
  const [roleId, setRoleId] = useState("");
  const roles = useRecordRoles().data;
  const create = useCreateRecordAssignment(recordType, recordId);
  const update = useUpdateRecordAssignment(recordType, recordId);
  const write = existing ? update : create;
  // Pulled out because the effect below depends on THESE functions rather than
  // on the mutation objects, which are new objects every render: depending on
  // the objects would reset the form under the reader mid-edit.
  const resetCreate = create.reset;
  const resetUpdate = update.reset;

  // Re-seeded on every opening, not once at mount. The modal stays mounted for
  // the life of the panel, so an initializer would answer a question about
  // whichever row was being edited the first time the panel rendered.
  useEffect(() => {
    if (!open) {
      return;
    }
    setSubjectKind(existing?.subject_kind ?? "user");
    setSubject(
      existing
        ? { id: existing.subject_id, name: existing.subject_name }
        : null,
    );
    setRoleId(existing?.role_id ?? "");
    resetCreate();
    resetUpdate();
  }, [open, existing, resetCreate, resetUpdate]);

  const offered = useMemo(
    () => assignableRoles(roles, recordType, subjectKind),
    [roles, recordType, subjectKind],
  );
  const options = useMemo(
    () => offered.map((role) => ({ value: role.id, label: role.label })),
    [offered],
  );

  // Kept on the kind alone. RecordPicker treats a new `searchTargets` identity
  // as a new search space and clears what it is showing, so anything that
  // changes while the reader types would empty the list under them.
  const searchTargets = useCallback(
    (q: string) => searchSubjects(subjectKind, q),
    [subjectKind],
  );

  function close() {
    onClose();
  }

  function pickKind(next: AssignmentSubjectKind) {
    setSubjectKind(next);
    setSubject(null);
    // A role applicable to a colleague need not be applicable to a team. Left
    // standing it would submit a combination the server refuses, so the reader
    // restates it against the list that now applies.
    setRoleId("");
  }

  async function submit() {
    if (!subject || !roleId) {
      return;
    }
    if (existing) {
      await update.mutateAsync({
        id: existing.id,
        body: {
          subject_kind: subjectKind,
          subject_id: subject.id,
          role_id: roleId,
        },
      });
    } else {
      await create.mutateAsync({
        subject_kind: subjectKind,
        subject_id: subject.id,
        role_id: roleId,
      });
    }
    close();
  }

  return (
    <Modal open={open} onClose={close} labelledBy={headingId}>
      <h2
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {existing ? t("assignments.changeTitle") : t("assignments.addTitle")}
      </h2>
      <div className="form-stack">
        <SegmentedControl
          label={t("assignments.subjectKind")}
          options={SUBJECT_KINDS}
          value={subjectKind}
          onChange={pickKind}
          labels={{
            user: t("assignments.kindUser"),
            team: t("assignments.kindTeam"),
          }}
        />
        <div className="field">
          <span className="t-label">{t("assignments.who")}</span>
          <RecordPicker
            label={
              subjectKind === "team"
                ? t("assignments.findTeam")
                : t("assignments.findColleague")
            }
            searchTargets={searchTargets}
            selected={subject}
            onPick={setSubject}
            disabled={write.isPending}
          />
        </div>
        <div className="field">
          <span className="t-label" id={`${headingId}-role`}>
            {t("assignments.role")}
          </span>
          <Select
            aria-labelledby={`${headingId}-role`}
            options={options}
            value={roleId}
            onChange={setRoleId}
            placeholder={t("assignments.rolePlaceholder")}
            disabled={write.isPending || options.length === 0}
          />
        </div>
        <p className="t-caption mute">{t("assignments.noAccessNote")}</p>
        {write.isError && (
          <p className="t-caption" role="alert">
            {problemMessageOf(write.error, t)}
          </p>
        )}
        <div className="actions">
          <Button variant="ghost" onClick={close} disabled={write.isPending}>
            {t("deals.cancel")}
          </Button>
          <Button
            onClick={() => void submit()}
            disabled={write.isPending || !subject || roleId === ""}
          >
            {existing ? t("assignments.saveChange") : t("assignments.saveAdd")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

/**
 * Colleagues and teams, as pickable candidates.
 *
 * Both lists are small and administered, so they are fetched whole and matched
 * here rather than through a search endpoint neither of them has. A caller
 * typing narrows what is already in hand.
 */
async function searchSubjects(
  kind: AssignmentSubjectKind,
  q: string,
): Promise<RecordPickerCandidate[]> {
  const term = q.trim().toLowerCase();
  if (kind === "team") {
    const { data, error, response } = await api.GET("/teams");
    if (error || !response.ok) {
      throwProblem(error);
    }
    return data.data
      .map((team) => ({ id: team.id, name: team.name }))
      .filter((team) => team.name.toLowerCase().includes(term));
  }
  const { data, error, response } = await api.GET("/users");
  if (error || !response.ok) {
    throwProblem(error);
  }
  return (
    data.data
      // Only a colleague who can actually be reached about the work. An agent
      // seat holds no responsibility a human can be asked about, and a suspended
      // or never-redeemed seat is not somebody to hand a record to.
      .filter((user) => user.status === "active" && !user.is_agent)
      .map((user) => ({ id: user.id, name: user.display_name }))
      .filter((user) => user.name.toLowerCase().includes(term))
  );
}
