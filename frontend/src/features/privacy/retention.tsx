import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../../api/client";
import {
  Badge,
  Button,
  EmptyState,
  Modal,
  Skeleton,
} from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { CellStrip } from "../../design-system/listtable";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, QueryStates } from "../../design-system/querystates";
import { RowOpen } from "../../design-system/rowopen";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import {
  actionLabelKey,
  actionTone,
  effectBadge,
  effectReasonKey,
  keepFor,
  policyEffect,
  RETENTION_POLICIES_KEY,
  RETENTION_SCOPES,
  RETENTION_SETTINGS_KEY,
  type RetentionPolicy,
  scopeLabelKey,
} from "./retention.logic";
import { RetentionPolicyForm } from "./retentionpolicyform";
import "./retention.css";

// The storage-limitation ladder and the retain-only posture that overrides its
// destructive half: an enabled policy can be inert, so a row not acting says so.
import {
  DeletePolicyModal,
  type EditorState,
  PolicyEditor,
} from "./retentionpolicyeditor";

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
      return unwrap(
        await api.PATCH("/retention/settings", {
          body: { retain_only: next },
        }),
      );
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
      return unwrap(await api.GET("/retention/settings"));
    },
  });

  const policies = useQuery({
    queryKey: RETENTION_POLICIES_KEY,
    enabled: canRead,
    queryFn: async () => {
      return unwrap(await api.GET("/retention-policies"));
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

  // In the authorable enum's order, so the ladder reads the same every visit.
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
      key: "open",
      header: t("table.actions"),
      headerHidden: true,
      fold: "end",
      align: "end",
      render: (policy) => (
        <RowOpen
          label={t("retention.editScope", {
            scope: t(scopeLabelKey(policy.scope)),
          })}
          opensDialog
          onOpen={() => onEdit(policy)}
        />
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
      onRowClick={canEdit ? onEdit : undefined}
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
