import { useState } from "react";
import { useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  Checkbox,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate } from "./common";
import {
  type AssignmentRecordType,
  type AssignmentSubjectKind,
  type RecordRole,
  useCreateRecordRole,
  useRecordRoles,
  useUpdateRecordRole,
} from "./recordassignments.queries";

/**
 * Settings → the responsibilities a colleague or team can hold on a record.
 *
 * A role names what somebody is accountable for and grants no access of its
 * own, which the subtitle says because "role" reads like a permission on most
 * products a user has met.
 *
 * There is no delete. A role an assignment has ever carried must stay
 * resolvable or that assignment stops rendering its own history, so the only
 * withdrawal is the active switch — which keeps the entry readable everywhere
 * it is already stored while removing it from every picker.
 *
 * Applicability is chosen at creation and read-only on existing roles. The
 * server refuses narrowing while a live assignment depends on the removed
 * scope; retiring a role preserves those assignments and permits a replacement.
 */
export function RecordRolesCard() {
  const t = useT();
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const query = useRecordRoles();
  const create = useCreateRecordRole();
  const update = useUpdateRecordRole();
  const [adding, setAdding] = useState(false);
  const failure = [create, update].find((m) => m.isError);
  return (
    <Panel
      title={t("recordRoles.title")}
      titleAction={
        canCreate && (
          <Button small onClick={() => setAdding(true)}>
            {t("recordRoles.addOpen")}
          </Button>
        )
      }
    >
      <PanelBody>
        <p className="settings-panel-sub">{t("recordRoles.sub")}</p>
        <SettingList>
          <SettingRow
            label={t("recordRoles.listLabel")}
            layout="stack"
            control={
              <QueryGate query={query} pendingLabel={t("recordRoles.loading")}>
                {(roles) => (
                  <ul
                    className="lead-vocab-list"
                    data-testid="record-role-list"
                  >
                    {roles.map((role) => (
                      <RecordRoleRow
                        key={role.id}
                        role={role}
                        canEdit={canEdit}
                        onUpdate={(body) =>
                          update.mutate({ id: role.id, body })
                        }
                      />
                    ))}
                  </ul>
                )}
              </QueryGate>
            }
          />
        </SettingList>
        {!canEdit && <p className="t-caption">{t("recordRoles.readOnly")}</p>}
        {failure?.error && (
          <Callout
            tone="danger"
            live="alert"
            title={problemMessageOf(failure.error, t)}
          />
        )}
        {adding && (
          <AddRecordRoleDialog
            onClose={() => setAdding(false)}
            onAdd={(body) =>
              create.mutate(body, { onSuccess: () => setAdding(false) })
            }
            pending={create.isPending}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

function RecordRoleRow({
  role,
  canEdit,
  onUpdate,
}: Readonly<{
  role: RecordRole;
  canEdit: boolean;
  onUpdate: (body: { label?: string; active?: boolean }) => void;
}>) {
  const t = useT();
  const [label, setLabel] = useState(role.label);
  return (
    <li className="lead-vocab-row" data-testid={`record-role-${role.key}`}>
      <Field label={t("recordRoles.labelFor", { key: role.key })}>
        {(control) => (
          <TextInput
            {...control}
            value={label}
            disabled={!canEdit}
            onChange={(e) => setLabel(e.target.value)}
            onBlur={() => {
              const next = label.trim();
              // Only on a real change, and never on an emptied field: a blank
              // box is a half-finished edit, not an instruction to erase the
              // name every assignment carrying this role renders through.
              if (next && next !== role.label) {
                onUpdate({ label: next });
              } else if (!next) {
                setLabel(role.label);
              }
            }}
          />
        )}
      </Field>
      {/* Where the role may be held, and by whom. Read-only, because narrowing
          either set is refused while a live assignment depends on it. */}
      <span className="t-caption mute">
        {role.record_types.join(", ")} · {role.assignee_kinds.join(", ")}
      </span>
      <span className="lead-vocab-flags">
        {role.system && <Badge>{t("recordRoles.builtIn")}</Badge>}
        <Switch
          label={t("recordRoles.activeFor", { label: role.label })}
          labelHidden
          checked={role.active}
          disabled={!canEdit}
          onChange={(next: boolean) => onUpdate({ active: next })}
        />
      </span>
    </li>
  );
}

function AddRecordRoleDialog({
  onClose,
  onAdd,
  pending,
}: Readonly<{
  onClose: () => void;
  onAdd: (body: {
    label: string;
    record_types: AssignmentRecordType[];
    assignee_kinds: AssignmentSubjectKind[];
  }) => void;
  pending: boolean;
}>) {
  const t = useT();
  const [label, setLabel] = useState("");
  const [recordTypes, setRecordTypes] = useState<AssignmentRecordType[]>([]);
  const [assigneeKinds, setAssigneeKinds] = useState<AssignmentSubjectKind[]>(
    [],
  );
  return (
    <Modal open onClose={onClose} labelledBy="record-role-add-title">
      <Heading
        size="large"
        className="t-h3 modal-title"
        id="record-role-add-title"
      >
        {t("recordRoles.addTitle")}
      </Heading>
      <Field label={t("recordRoles.addLabel")} hint={t("recordRoles.addHint")}>
        {(control) => (
          <TextInput
            {...control}
            value={label}
            onChange={(e) => setLabel(e.target.value)}
          />
        )}
      </Field>
      <fieldset className="field-multiselect" disabled={pending}>
        <legend className="t-label">{t("recordRoles.recordTypes")}</legend>
        {(["company", "deal", "project"] as const).map((kind) => (
          <Checkbox
            key={kind}
            label={t(`search.kind.${kind}`)}
            checked={recordTypes.includes(kind)}
            onChange={(e) =>
              setRecordTypes((current) =>
                e.target.checked
                  ? [...current, kind]
                  : current.filter((value) => value !== kind),
              )
            }
          />
        ))}
      </fieldset>
      <fieldset className="field-multiselect" disabled={pending}>
        <legend className="t-label">{t("recordRoles.assigneeKinds")}</legend>
        {(["user", "team"] as const).map((kind) => (
          <Checkbox
            key={kind}
            label={t(
              kind === "user" ? "assignments.kindUser" : "assignments.kindTeam",
            )}
            checked={assigneeKinds.includes(kind)}
            onChange={(e) =>
              setAssigneeKinds((current) =>
                e.target.checked
                  ? [...current, kind]
                  : current.filter((value) => value !== kind),
              )
            }
          />
        ))}
      </fieldset>
      <div className="action-row">
        <Button variant="ghost" onClick={onClose}>
          {t("deals.cancel")}
        </Button>
        <Button
          disabled={
            !label.trim() ||
            !recordTypes.length ||
            !assigneeKinds.length ||
            pending
          }
          onClick={() =>
            onAdd({
              label: label.trim(),
              record_types: recordTypes,
              assignee_kinds: assigneeKinds,
            })
          }
        >
          {t("recordRoles.addConfirm")}
        </Button>
      </div>
    </Modal>
  );
}
