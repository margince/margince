import { useState } from "react";
import { useCanWrite } from "../app/capability";
import { Badge, Button, Field, Modal, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate } from "./common";
import {
  type RecordRole,
  useCreateRecordRole,
  useRecordRoles,
  useUpdateRecordRole,
} from "./recordassignments.queries";

/**
 * Settings → the responsibilities a person or team can hold on a record.
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
 * Applicability is shown but not edited here. Narrowing it is refused by the
 * server while a live assignment depends on what would be removed, and a
 * control whose save usually fails is worse than none: retiring the role and
 * adding its replacement is the move that always works.
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
            onAdd={(label) =>
              create.mutate(
                {
                  label,
                  // The widest applicability a new role can have, narrowed
                  // afterwards by retiring and replacing rather than by editing
                  // — which is the only direction the server accepts once an
                  // assignment depends on it.
                  record_types: ["company", "deal", "project"],
                  assignee_kinds: ["user", "team"],
                },
                { onSuccess: () => setAdding(false) },
              )
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
  onAdd: (label: string) => void;
  pending: boolean;
}>) {
  const t = useT();
  const [label, setLabel] = useState("");
  return (
    <Modal open onClose={onClose} labelledBy="record-role-add-title">
      <h2 className="t-h3 modal-title" id="record-role-add-title">
        {t("recordRoles.addTitle")}
      </h2>
      <Field label={t("recordRoles.addLabel")} hint={t("recordRoles.addHint")}>
        {(control) => (
          <TextInput
            {...control}
            value={label}
            onChange={(e) => setLabel(e.target.value)}
          />
        )}
      </Field>
      <div className="action-row">
        <Button variant="ghost" onClick={onClose}>
          {t("deals.cancel")}
        </Button>
        <Button
          disabled={!label.trim() || pending}
          onClick={() => onAdd(label.trim())}
        >
          {t("recordRoles.addConfirm")}
        </Button>
      </div>
    </Modal>
  );
}
