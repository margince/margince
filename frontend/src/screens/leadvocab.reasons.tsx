// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { NameDialog } from "../design-system/namedialog";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Switch } from "../design-system/switch";
import { formatNumber } from "../format/format";
import {
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { Locale } from "../i18n/locale";
import { problemMessageOf, QueryStates, unwrap } from "./common";
import {
  LEAD_DISQUALIFY_REASONS_KEY,
  type LeadDisqualifyReason,
  useLeadDisqualifyReasons,
} from "./leadsources";
import {
  LeadCount,
  nameRefusal,
  rowsOf,
  VocabNotices,
  VocabRowMenu,
} from "./leadvocab.rows";

type ReasonPatch = { id: string; label?: string; active?: boolean };

async function patchReason({ id, ...body }: ReasonPatch) {
  return unwrap(
    await api.PATCH("/lead-disqualify-reasons/{id}", {
      params: { path: { id } },
      body,
    }),
  );
}

// The name dialog owns create and rename, so their refusals stay out of the card.
function useReasonMutations() {
  const queryClient = useQueryClient();
  const invalidate = () => {
    void queryClient.invalidateQueries({
      queryKey: LEAD_DISQUALIFY_REASONS_KEY,
    });
  };
  const create = useMutation({
    mutationFn: async (body: { label: string }) => {
      return unwrap(await api.POST("/lead-disqualify-reasons", { body }));
    },
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: patchReason,
    onSuccess: invalidate,
  });
  const rename = useMutation({
    mutationFn: patchReason,
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.DELETE("/lead-disqualify-reasons/{id}", {
          params: { path: { id } },
        }),
      );
    },
    onSuccess: invalidate,
  });
  return { create, update, rename, remove };
}

function reasonColumns({
  t,
  plural,
  locale,
  canEdit,
  canRemove,
  onActive,
  onRename,
  onRemove,
}: Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  canEdit: boolean;
  canRemove: boolean;
  onActive: (reason: LeadDisqualifyReason, active: boolean) => void;
  onRename: (reason: LeadDisqualifyReason) => void;
  onRemove: (reason: LeadDisqualifyReason) => void;
}>): DataTableColumn<LeadDisqualifyReason>[] {
  const refusal = (reason: LeadDisqualifyReason) => {
    const count = reason.lead_count ?? 0;
    if (reason.system === true) return t("leadReasons.builtInKept");
    if (count > 0) {
      return plural("leadReasons.inUse", count, {
        count: formatNumber(count, locale),
      });
    }
    return undefined;
  };
  return [
    {
      key: "name",
      header: t("leadReasons.colReason"),
      grow: true,
      render: (reason) => (
        <span className="lead-vocab-name">
          <span>{reason.label}</span>
          {canRemove && reason.system === true && (
            <Badge>{t("leadSources.builtIn")}</Badge>
          )}
        </span>
      ),
    },
    {
      key: "leads",
      header: t("leadSources.colLeads"),
      align: "end",
      render: (reason) => <LeadCount count={reason.lead_count ?? 0} />,
    },
    {
      key: "active",
      header: t("leadSources.colActive"),
      render: (reason) => (
        <Switch
          label={t("leadSources.activeFor", { label: reason.label })}
          labelHidden
          checked={reason.active}
          disabled={!canEdit}
          onChange={(next) => onActive(reason, next)}
        />
      ),
    },
    {
      key: "verbs",
      header: t("leadSources.colActions"),
      headerHidden: true,
      fold: "end",
      render: (reason) => (
        <VocabRowMenu
          label={reason.label}
          canEdit={canEdit}
          canRemove={canRemove}
          refusal={refusal(reason)}
          onRename={() => onRename(reason)}
          onRemove={() => onRemove(reason)}
        />
      ),
    },
  ];
}

// A reason is one sentence, so creating and renaming share the name dialog.
type Naming =
  | { mode: "create" }
  | { mode: "rename"; reason: LeadDisqualifyReason };

export function LeadDisqualifyReasonsCard() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const canRemove = useCanWrite("custom_field", "delete");
  const query = useLeadDisqualifyReasons();
  const { create, update, rename, remove } = useReasonMutations();
  // Kept after close, so the dialog leaving the page still reads as what it was.
  const [naming, setNaming] = useState<Naming>({ mode: "create" });
  const [namingOpen, setNamingOpen] = useState(false);
  const [removing, setRemoving] = useState<LeadDisqualifyReason | null>(null);
  const reasons = rowsOf(query.data);
  const write = naming.mode === "rename" ? rename : create;
  const refused = nameRefusal(write.error, t, "leadReasons.duplicate");
  const open = (next: Naming) => {
    create.reset();
    rename.reset();
    setNaming(next);
    setNamingOpen(true);
  };
  const columns = reasonColumns({
    t,
    plural,
    locale,
    canEdit,
    canRemove,
    onActive: (reason, active) => update.mutate({ id: reason.id, active }),
    onRename: (reason) => open({ mode: "rename", reason }),
    onRemove: setRemoving,
  });
  const done = { onSuccess: () => setNamingOpen(false) };
  return (
    <Panel
      title={t("leadReasons.title")}
      titleAction={
        canCreate && (
          <Button onClick={() => open({ mode: "create" })}>
            {t("leadReasons.newLabel")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("leadReasons.sub")}</PanelIntro>
      </PanelBody>
      {query.isSuccess && reasons.length > 0 ? (
        <DataTable
          bleed
          fold
          label={t("leadReasons.title")}
          columns={columns}
          rows={[...reasons]}
          rowKey={(reason) => reason.id}
          rowTestId={(reason) => `lead-reason-${reason.id}`}
        />
      ) : (
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("leadReasons.title")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      <VocabNotices
        readOnly={!canEdit}
        error={update.isError ? update.error : undefined}
      />
      <NameDialog
        open={namingOpen}
        onClose={() => setNamingOpen(false)}
        title={t(
          naming.mode === "rename"
            ? "leadReasons.renameTitle"
            : "leadReasons.newLabel",
        )}
        label={t("leadReasons.labelField")}
        initial={naming.mode === "rename" ? naming.reason.label : ""}
        confirmLabel={t(
          naming.mode === "rename"
            ? "leadSources.renameSave"
            : "leadReasons.add",
        )}
        pending={write.isPending}
        problem={refused.problem}
        nameProblem={refused.nameProblem}
        onSave={(label) => {
          if (naming.mode === "rename") {
            rename.mutate({ id: naming.reason.id, label }, done);
          } else {
            create.mutate({ label }, done);
          }
        }}
      />
      <ConfirmModal
        open={removing !== null}
        onClose={() => {
          remove.reset();
          setRemoving(null);
        }}
        title={t("leadReasons.removeTitle")}
        confirmLabel={t("leadSources.remove")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        onConfirm={() => {
          if (removing) {
            remove.mutate(removing.id, { onSuccess: () => setRemoving(null) });
          }
        }}
      >
        <p>{t("leadReasons.removeBody", { label: removing?.label ?? "" })}</p>
      </ConfirmModal>
    </Panel>
  );
}
