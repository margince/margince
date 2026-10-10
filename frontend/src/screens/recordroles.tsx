// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, useState } from "react";
import { rowsOf } from "../api/rows";
import { useCanWrite } from "../app/capability";
import {
  Button,
  Checkbox,
  EmptyState,
  Field,
  OverflowMenu,
  TextInput,
} from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { KeyedName } from "../design-system/keyedname";
import {
  NameDialog,
  nameRefusal,
  useRefusedName,
} from "../design-system/namedialog";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { PanelNotices } from "../design-system/panelnotices";
import { useSinglePress } from "../design-system/presslatch";
import { Switch } from "../design-system/switch";
import { INTL_LOCALE } from "../format/format";
import { type Translator, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { QueryStates } from "./common";
import {
  type AssignmentRecordType,
  type AssignmentSubjectKind,
  type RecordRole,
  useCreateRecordRole,
  useRecordRoles,
  useUpdateRecordRole,
} from "./recordassignments.queries";

const RECORD_KINDS: Record<AssignmentRecordType, MessageKey> = {
  company: "recordRoles.kind.company",
  deal: "recordRoles.kind.deal",
  project: "recordRoles.kind.project",
};

function heldByKey(kinds: readonly AssignmentSubjectKind[]): MessageKey | null {
  const user = kinds.includes("user");
  const team = kinds.includes("team");
  if (user && team) return "recordRoles.heldBy.either";
  if (user) return "recordRoles.heldBy.user";
  return team ? "recordRoles.heldBy.team" : null;
}

// Read-only, because narrowing either set is refused while a live assignment
// depends on it.
function RoleReach({ role }: Readonly<{ role: RecordRole }>) {
  const t = useT();
  const { locale } = useLocale();
  const records = new Intl.ListFormat(INTL_LOCALE[locale], {
    style: "long",
    type: "conjunction",
  }).format(rowsOf(role.record_types).map((kind) => t(RECORD_KINDS[kind])));
  const heldBy = heldByKey(rowsOf(role.assignee_kinds));
  return (
    <CellStack>
      <span>{t("recordRoles.appliesOn", { records })}</span>
      {heldBy && <span className="t-caption">{t(heldBy)}</span>}
    </CellStack>
  );
}

function roleColumns({
  t,
  canEdit,
  onActive,
  onRename,
}: Readonly<{
  t: Translator;
  canEdit: boolean;
  onActive: (role: RecordRole, active: boolean) => void;
  onRename: (role: RecordRole) => void;
}>): DataTableColumn<RecordRole>[] {
  const columns: DataTableColumn<RecordRole>[] = [
    {
      key: "name",
      header: t("recordRoles.colRole"),
      grow: true,
      render: (role) => <KeyedName name={role.label} code={role.key} />,
    },
    {
      key: "reach",
      header: t("recordRoles.recordTypes"),
      render: (role) => <RoleReach role={role} />,
    },
    {
      key: "active",
      header: t("leadSources.colActive"),
      render: (role) => (
        <Switch
          label={t("recordRoles.activeFor", { label: role.label })}
          labelHidden
          checked={role.active}
          disabled={!canEdit}
          onChange={(next) => onActive(role, next)}
        />
      ),
    },
  ];
  if (canEdit) {
    columns.push({
      key: "verbs",
      header: t("leadSources.colActions"),
      headerHidden: true,
      fold: "end",
      render: (role) => (
        <span className="cell-actions">
          <OverflowMenu label={t("table.rowActions", { name: role.label })}>
            <Button onClick={() => onRename(role)}>
              {t("leadSources.rename")}
            </Button>
          </OverflowMenu>
        </span>
      ),
    });
  }
  return columns;
}

function Choices<Value extends string>({
  legend,
  options,
  chosen,
  disabled,
  onChange,
}: Readonly<{
  legend: string;
  options: readonly { value: Value; label: string }[];
  chosen: readonly Value[];
  disabled: boolean;
  onChange: (next: Value[]) => void;
}>) {
  return (
    <fieldset className="field-multiselect" disabled={disabled}>
      <legend className="t-name">{legend}</legend>
      {options.map(({ value, label }) => (
        <Checkbox
          key={value}
          label={label}
          checked={chosen.includes(value)}
          onChange={(e) =>
            onChange(
              e.target.checked
                ? [...chosen, value]
                : chosen.filter((other) => other !== value),
            )
          }
        />
      ))}
    </fieldset>
  );
}

// Mounted only while open, so a half-made role never waits for the next opening.
function AddRoleDialog({
  create,
  onClose,
}: Readonly<{
  create: ReturnType<typeof useCreateRecordRole>;
  onClose: () => void;
}>) {
  const t = useT();
  const [label, setLabel] = useState("");
  const [recordTypes, setRecordTypes] = useState<AssignmentRecordType[]>([]);
  const [assigneeKinds, setAssigneeKinds] = useState<AssignmentSubjectKind[]>(
    [],
  );
  const name = label.trim();
  const refused = nameRefusal(create.error, t, "recordRoles.duplicate");
  const { refusedName: nameProblem, markSent } = useRefusedName(
    name,
    refused.nameProblem,
  );
  const singlePress = useSinglePress(create.isPending);
  const ready =
    name !== "" &&
    recordTypes.length > 0 &&
    assigneeKinds.length > 0 &&
    !nameProblem;
  const save = () => {
    if (!ready || create.isPending) return;
    markSent();
    create.mutate(
      {
        label: name,
        record_types: recordTypes,
        assignee_kinds: assigneeKinds,
      },
      { onSuccess: onClose },
    );
  };
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("recordRoles.addTitle")}
      intent="form"
      confirmLabel={t("recordRoles.addConfirm")}
      confirmDisabled={!ready}
      pending={create.isPending}
      error={refused.problem}
      onConfirm={save}
    >
      <form
        className="form-stack"
        onSubmit={singlePress((event: FormEvent) => {
          event.preventDefault();
          save();
        })}
      >
        <Field
          label={t("recordRoles.addLabel")}
          hint={t("recordRoles.addHint")}
          error={nameProblem}
        >
          {(control) => (
            <TextInput
              {...control}
              value={label}
              onChange={(e) => setLabel(e.target.value)}
            />
          )}
        </Field>
        <Choices
          legend={t("recordRoles.recordTypes")}
          options={(["company", "deal", "project"] as const).map((kind) => ({
            value: kind,
            label: t(`search.kind.${kind}`),
          }))}
          chosen={recordTypes}
          disabled={create.isPending}
          onChange={setRecordTypes}
        />
        <Choices
          legend={t("recordRoles.assigneeKinds")}
          options={[
            { value: "user", label: t("assignments.kindUser") },
            { value: "team", label: t("assignments.kindTeam") },
          ]}
          chosen={assigneeKinds}
          disabled={create.isPending}
          onChange={setAssigneeKinds}
        />
      </form>
    </ConfirmModal>
  );
}

// A role names what somebody answers for on a record and grants no access.
// No delete: a role an assignment carried must stay resolvable, so the switch
// retires it. Where a role applies is set at creation only.
export function RecordRolesCard() {
  const t = useT();
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const query = useRecordRoles();
  const create = useCreateRecordRole();
  const update = useUpdateRecordRole();
  const rename = useUpdateRecordRole();
  const [adding, setAdding] = useState(false);
  // Kept after close, so the dialog leaving the page still names its role.
  const [renaming, setRenaming] = useState<RecordRole | null>(null);
  const [renamingOpen, setRenamingOpen] = useState(false);
  const roles = rowsOf(query.data);
  const refused = nameRefusal(rename.error, t, "recordRoles.duplicate");
  const columns = roleColumns({
    t,
    canEdit,
    onActive: (role, active) =>
      update.mutate({ id: role.id, body: { active } }),
    onRename: (role) => {
      rename.reset();
      setRenaming(role);
      setRenamingOpen(true);
    },
  });
  return (
    <Panel
      title={t("recordRoles.title")}
      titleAction={
        canCreate && (
          <Button onClick={() => setAdding(true)}>
            {t("recordRoles.addOpen")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("recordRoles.sub")}</PanelIntro>
      </PanelBody>
      {query.isSuccess && roles.length > 0 ? (
        <DataTable
          bleed
          fold
          label={t("recordRoles.title")}
          columns={columns}
          rows={[...roles]}
          rowKey={(role) => role.id}
          rowTestId={(role) => `record-role-${role.key}`}
        />
      ) : (
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("recordRoles.loading")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      <PanelNotices
        readOnly={!canEdit && t("leadSources.readOnlyTitle")}
        refused={
          update.isError
            ? { title: t("leadSources.notSaved"), error: update.error }
            : undefined
        }
      />
      {adding && (
        <AddRoleDialog
          create={create}
          onClose={() => {
            create.reset();
            setAdding(false);
          }}
        />
      )}
      <NameDialog
        open={renamingOpen}
        onClose={() => setRenamingOpen(false)}
        title={t("recordRoles.renameTitle")}
        label={t("recordRoles.addLabel")}
        initial={renaming?.label ?? ""}
        confirmLabel={t("leadSources.renameSave")}
        pending={rename.isPending}
        problem={refused.problem}
        nameProblem={refused.nameProblem}
        onSave={(label) => {
          if (renaming) {
            rename.mutate(
              { id: renaming.id, body: { label } },
              { onSuccess: () => setRenamingOpen(false) },
            );
          }
        }}
      />
    </Panel>
  );
}
