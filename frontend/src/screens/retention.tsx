import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import { useCan, useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Modal,
  Skeleton,
  TextInput,
} from "../design-system/atoms";
import { CardBoundary } from "../design-system/cardboundary";
import { CellStack } from "../design-system/cellstack";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { CellStrip } from "../design-system/listtable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import {
  problemMessageOf,
  QueryGate,
  QueryStates,
  throwProblem,
  useMe,
} from "./common";
import {
  actionLabelKey,
  actionTone,
  effectBadge,
  effectReasonKey,
  keepFor,
  parseRetainDays,
  policyEffect,
  RETENTION_ACTIONS,
  RETENTION_POLICIES_KEY,
  RETENTION_SCOPES,
  RETENTION_SETTINGS_KEY,
  type RetentionAction,
  type RetentionPolicy,
  scopeLabelKey,
} from "./retention.logic";
import { RetentionPolicyForm } from "./retentionpolicyform";
import "./retention.css";

// The storage-limitation ladder and the retain-only posture that overrides its
// destructive half: an enabled policy can be inert, so a row not acting says so.

// A row's two writes are one PATCH on one policy, so they stay one mutation;
// `intent` says which of the two it is, because only a save closes the editor.
type PolicyWrite = Readonly<{
  intent: "save" | "switch";
  body: Readonly<{
    retain_days?: number;
    action?: RetentionAction;
    lawful_basis?: string | null;
    enabled?: boolean;
  }>;
}>;

// Mounted once per opening (keyed on the session), so the fields always start
// from the policy as it now stands rather than from an abandoned draft.
function PolicyEditorBody({
  policy,
  titleId,
  canDelete,
  onClose,
  onDelete,
}: Readonly<{
  policy: RetentionPolicy;
  titleId: string;
  canDelete: boolean;
  onClose: () => void;
  onDelete: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [retainDays, setRetainDays] = useState(String(policy.retain_days));
  const [action, setAction] = useState<RetentionAction>(policy.action);
  const [lawfulBasis, setLawfulBasis] = useState(policy.lawful_basis ?? "");
  const days = parseRetainDays(retainDays);

  const patch = useMutation({
    mutationFn: async ({ body }: PolicyWrite) => {
      const { data, error } = await api.PATCH("/retention-policies/{id}", {
        params: { path: { id: policy.id } },
        body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (_data, write) => {
      queryClient.invalidateQueries({ queryKey: RETENTION_POLICIES_KEY });
      // The switch leaves the editor open: closing it would drop the fields
      // the operator may be mid-edit on, and focus with them.
      if (write.intent === "save") {
        onClose();
      }
    },
  });

  return (
    <>
      {/* The scope names which policy is open: the dialog covers its row. */}
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {t(scopeLabelKey(policy.scope))}
      </Heading>
      <div className="form-stack">
        <Field
          label={t("retention.window")}
          hint={
            days === null && retainDays.trim() !== ""
              ? t("retention.windowInvalid")
              : undefined
          }
        >
          {(control) => (
            <TextInput
              {...control}
              inputMode="numeric"
              value={retainDays}
              onChange={(event) => setRetainDays(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("retention.action")}>
          {(control) => (
            <Select
              {...control}
              options={RETENTION_ACTIONS.map((value) => ({
                value,
                label: t(actionLabelKey(value)),
              }))}
              value={action}
              onChange={(value) => {
                const picked = RETENTION_ACTIONS.find(
                  (candidate) => candidate === value,
                );
                if (picked) {
                  setAction(picked);
                }
              }}
            />
          )}
        </Field>
        <Field label={t("retention.lawfulBasis")}>
          {(control) => (
            <TextInput
              {...control}
              value={lawfulBasis}
              onChange={(event) => setLawfulBasis(event.target.value)}
            />
          )}
        </Field>
        {/* Flipping it is the pause, with no Save after it. */}
        <Switch
          label={t("retention.enabled")}
          checked={policy.enabled}
          pending={patch.isPending && patch.variables?.intent === "switch"}
          onChange={(next) =>
            patch.mutate({ intent: "switch", body: { enabled: next } })
          }
        />
        <ErrorLine error={patch.error} />
      </div>
      <div className="actions">
        {/* The confirm replaces this dialog: two dialogs trap focus in the
            wrong one and share one Escape key. */}
        {canDelete && (
          <span className="actions-lead">
            <Button variant="danger" onClick={onDelete}>
              {t("retention.delete")}
            </Button>
          </span>
        )}
        <span className="actions-pair">
          <Button onClick={onClose}>{t("deals.cancel")}</Button>
          <Button
            variant="primary"
            disabled={days === null || patch.isPending}
            onClick={() =>
              days !== null &&
              patch.mutate({
                intent: "save",
                body: {
                  retain_days: days,
                  action,
                  lawful_basis: lawfulBasis.trim() || null,
                },
              })
            }
          >
            {t("retention.save")}
          </Button>
        </span>
      </div>
    </>
  );
}

type EditorState = Readonly<{ id: string; open: boolean; session: number }>;

function PolicyEditor({
  editor,
  policies,
  canDelete,
  onClose,
  onDelete,
}: Readonly<{
  editor: EditorState | null;
  policies: readonly RetentionPolicy[];
  canDelete: boolean;
  onClose: () => void;
  onDelete: (policy: RetentionPolicy) => void;
}>) {
  const titleId = useId();
  // Read from the live list, so the Enabled switch shows what the server holds.
  const policy = policies.find((each) => each.id === editor?.id) ?? null;
  return (
    <Modal
      open={editor?.open === true && policy !== null}
      onClose={onClose}
      labelledBy={titleId}
      intent="form"
    >
      {policy && editor && (
        <PolicyEditorBody
          key={editor.session}
          policy={policy}
          titleId={titleId}
          canDelete={canDelete}
          onClose={onClose}
          onDelete={() => onDelete(policy)}
        />
      )}
    </Modal>
  );
}

// Deleting a policy is not the way to pause one: it drops the rule entirely,
// so records in that scope stop ageing out. The body says so and points at the
// Enabled switch, because that is what the operator usually meant.
function DeletePolicyModal({
  policy,
  onClose,
}: Readonly<{ policy: RetentionPolicy | null; onClose: () => void }>) {
  const t = useT();
  const queryClient = useQueryClient();

  const remove = useMutation({
    mutationFn: async (target: RetentionPolicy) => {
      const { error } = await api.DELETE("/retention-policies/{id}", {
        params: { path: { id: target.id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: RETENTION_POLICIES_KEY });
      onClose();
    },
  });

  function close() {
    onClose();
    remove.reset();
  }

  return (
    <ConfirmModal
      open={policy !== null}
      onClose={close}
      title={t("retention.deleteTitle")}
      confirmLabel={t("retention.delete")}
      confirmVariant="danger"
      onConfirm={() => policy && remove.mutate(policy)}
      pending={remove.isPending}
      error={remove.isError ? problemMessageOf(remove.error, t) : null}
    >
      <p>
        {policy
          ? t("retention.deleteBody", { scope: t(scopeLabelKey(policy.scope)) })
          : ""}
      </p>
    </ConfirmModal>
  );
}

// Refused, never hidden, without the update grant: every reader needs to know
// which posture is in force, and `reason` says why they cannot change it.
function PostureToggle({
  retainOnly,
  canManage,
}: Readonly<{ retainOnly: boolean; canManage: boolean }>) {
  const t = useT();
  const queryClient = useQueryClient();

  const update = useMutation({
    mutationFn: async (next: boolean) => {
      const { data, error } = await api.PATCH("/retention/settings", {
        body: { retain_only: next },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(RETENTION_SETTINGS_KEY, data);
      // Every row's `suppressed_by_posture` derives from the posture just written.
      queryClient.invalidateQueries({ queryKey: RETENTION_POLICIES_KEY });
    },
  });

  return (
    <div className="retention-posture">
      {/* The row draws the naming, so the switch carries the same words hidden. */}
      <Switch
        label={t("retention.retainOnly")}
        labelHidden
        reason={canManage ? undefined : t("retention.adminOnly")}
        checked={retainOnly}
        disabled={!canManage}
        pending={update.isPending}
        onChange={(next) => update.mutate(next)}
      />
      <ErrorLine error={update.error} />
    </div>
  );
}

export function RetentionCard() {
  const t = useT();
  const me = useMe();
  const canRead = useCan("retention_policy", "read");
  const canManage = useCanWrite("retention_policy", "update");
  const canCreate = useCanWrite("retention_policy", "create");
  const canDelete = useCanWrite("retention_policy", "delete");
  const addTitleId = useId();
  const [adding, setAdding] = useState(false);
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [deleting, setDeleting] = useState<RetentionPolicy | null>(null);

  const settings = useQuery({
    queryKey: RETENTION_SETTINGS_KEY,
    enabled: canRead,
    queryFn: async () => {
      const { data, error } = await api.GET("/retention/settings");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  const policies = useQuery({
    queryKey: RETENTION_POLICIES_KEY,
    enabled: canRead,
    queryFn: async () => {
      const { data, error } = await api.GET("/retention-policies");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  // Withheld, not absent, and no request: a vanished card reads as "keeps
  // everything", and a known 403 offers a Retry that cannot succeed.
  if (!canRead) {
    return (
      <Panel title={t("retention.title")}>
        <PanelBody>
          <PanelIntro>{t("retention.sub")}</PanelIntro>
          <QueryGate query={me} pendingLabel={t("retention.title")}>
            {() => <EmptyState>{t("retention.withheld")}</EmptyState>}
          </QueryGate>
        </PanelBody>
      </Panel>
    );
  }

  // Sorted by the authorable enum's order, so the ladder reads the same on
  // every visit and after every edit.
  const rows = [...(policies.data?.data ?? [])].sort(
    (left, right) =>
      RETENTION_SCOPES.indexOf(left.scope) -
      RETENTION_SCOPES.indexOf(right.scope),
  );

  return (
    <Panel
      title={t("retention.title")}
      titleAction={
        canCreate ? (
          <Button onClick={() => setAdding(true)}>
            {t("retention.addPolicy")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <PanelIntro>{t("retention.sub")}</PanelIntro>
      </PanelBody>
      <CardBoundary>
        {/* The posture first: a reader auditing the ladder needs the override
            before the rows it overrides. */}
        <SettingList bleed="settings">
          <SettingRow
            label={t("retention.retainOnly")}
            description={t("retention.retainOnlyHelp")}
            control={
              settings.isPending ? (
                <Skeleton width={40} height={22} />
              ) : settings.isError ? (
                <ErrorLine error={settings.error} />
              ) : (
                <PostureToggle
                  retainOnly={settings.data.retain_only}
                  canManage={canManage}
                />
              )
            }
          />
        </SettingList>
        {policies.isSuccess && rows.length > 0 ? (
          <PolicyTable
            rows={rows}
            canEdit={canManage}
            onEdit={(policy) =>
              setEditor((current) => ({
                id: policy.id,
                open: true,
                session: (current?.session ?? 0) + 1,
              }))
            }
          />
        ) : (
          <PanelBody>
            <QueryStates query={policies} pendingLabel={t("retention.title")}>
              <EmptyState>{t("retention.empty")}</EmptyState>
            </QueryStates>
          </PanelBody>
        )}
        <PolicyEditor
          editor={editor}
          policies={rows}
          canDelete={canDelete}
          onClose={() =>
            setEditor((current) => current && { ...current, open: false })
          }
          onDelete={(policy) => {
            setEditor((current) => current && { ...current, open: false });
            setDeleting(policy);
          }}
        />
        <Modal
          open={adding}
          onClose={() => setAdding(false)}
          labelledBy={addTitleId}
          intent="form"
        >
          <Heading size="large" id={addTitleId} className="t-h2 modal-title">
            {t("retention.addPolicy")}
          </Heading>
          <RetentionPolicyForm onDone={() => setAdding(false)} />
        </Modal>
        <DeletePolicyModal
          policy={deleting}
          onClose={() => setDeleting(null)}
        />
      </CardBoundary>
    </Panel>
  );
}

function PolicyTable({
  rows,
  canEdit,
  onEdit,
}: Readonly<{
  rows: readonly RetentionPolicy[];
  canEdit: boolean;
  onEdit: (policy: RetentionPolicy) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const columns: DataTableColumn<RetentionPolicy>[] = [
    {
      key: "scope",
      header: t("retention.colScope"),
      grow: true,
      render: (policy) => <ScopeCell policy={policy} />,
    },
    {
      key: "keep",
      header: t("retention.colKeep"),
      align: "end",
      render: (policy) => {
        const window = keepFor(policy.retain_days);
        return plural(window.unit, window.count, {
          count: formatNumber(window.count, locale),
        });
      },
    },
    {
      key: "then",
      header: t("retention.colThen"),
      render: (policy) => {
        const status = effectBadge(policyEffect(policy));
        return (
          <CellStrip>
            <Badge tone={actionTone(policy.action)}>
              {t(actionLabelKey(policy.action))}
            </Badge>
            {status && <Badge tone={status.tone}>{t(status.key)}</Badge>}
          </CellStrip>
        );
      },
    },
  ];
  if (canEdit) {
    columns.push({
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      fold: "end",
      align: "end",
      render: (policy) => (
        <span className="cell-actions">
          <Button
            aria-label={t("retention.editScope", {
              scope: t(scopeLabelKey(policy.scope)),
            })}
            aria-haspopup="dialog"
            onClick={() => onEdit(policy)}
          >
            {t("retention.edit")}
          </Button>
        </span>
      ),
    });
  }
  return (
    <DataTable
      label={t("retention.title")}
      bleed
      fold
      columns={columns}
      rows={[...rows]}
      rowKey={(policy) => policy.id}
    />
  );
}

// A disabled rule says it is kept and inert, so it never reads as deleted.
function ScopeCell({ policy }: Readonly<{ policy: RetentionPolicy }>) {
  const t = useT();
  const reasonKey = effectReasonKey(policyEffect(policy));
  const name: ReactNode = <span>{t(scopeLabelKey(policy.scope))}</span>;
  if (!reasonKey) {
    return name;
  }
  return (
    <CellStack>
      {name}
      <span className="t-caption">{t(reasonKey)}</span>
    </CellStack>
  );
}
