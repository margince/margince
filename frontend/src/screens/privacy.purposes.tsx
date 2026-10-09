// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  Checkbox,
  EmptyState,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { CardBoundary } from "../design-system/cardboundary";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useT } from "../i18n";
import { QueryStates, throwProblem, useMe } from "./common";
import "./privacy.css";

type ConsentPurpose = components["schemas"]["ConsentPurpose"];

type PurposeDraft = Readonly<{
  key: string;
  label: string;
  requiresDoi: boolean;
}>;

const EMPTY_DRAFT: PurposeDraft = { key: "", label: "", requiresDoi: false };

// Three inputs committed together. A stale create error must not outlive the
// edit that could fix it, so every change clears it first.
function PurposeCreateForm({ onDone }: Readonly<{ onDone: () => void }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<PurposeDraft>(EMPTY_DRAFT);
  const formId = useId();

  const create = useMutation({
    mutationFn: async (purpose: PurposeDraft) => {
      const { data, error } = await api.POST("/consent-purposes", {
        body: {
          key: purpose.key.trim(),
          label: purpose.label.trim(),
          requires_double_opt_in: purpose.requiresDoi,
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["consent-purposes"] });
      setDraft(EMPTY_DRAFT);
      onDone();
    },
  });

  function edit(next: Partial<PurposeDraft>) {
    setDraft((was) => ({ ...was, ...next }));
    if (create.isError) {
      create.reset();
    }
  }

  const ready = draft.key.trim() !== "" && draft.label.trim() !== "";
  return (
    <>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (ready && !create.isPending) create.mutate(draft);
        }}
      >
        <p>{t("privacy.purposeAppendOnly")}</p>
        <Field label={t("privacy.purposeKey")}>
          {(control) => (
            <TextInput
              {...control}
              value={draft.key}
              onChange={(event) => edit({ key: event.target.value })}
            />
          )}
        </Field>
        <Field label={t("privacy.purposeLabel")}>
          {(control) => (
            <TextInput
              {...control}
              value={draft.label}
              onChange={(event) => edit({ label: event.target.value })}
            />
          )}
        </Field>
        <Checkbox
          label={t("privacy.purposeDoi")}
          checked={draft.requiresDoi}
          onChange={(event) => edit({ requiresDoi: event.target.checked })}
        />
        <ErrorLine error={create.error} />
      </form>
      <div className="actions">
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={!ready || create.isPending}
        >
          {t("privacy.purposeCreate")}
        </Button>
      </div>
    </>
  );
}

export function ConsentPurposesCard() {
  const t = useT();
  // The probe itself: every grant reads false while /me is in flight.
  const me = useMe();
  // `useCanWrite`: the seat ceiling refuses a read seat's POST before RBAC.
  const canAdminister = useCanWrite("consent_config", "create");
  const addTitleId = useId();
  const [adding, setAdding] = useState(false);
  const query = useQuery({
    queryKey: ["consent-purposes"],
    queryFn: async () => {
      const { data, error } = await api.GET("/consent-purposes");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const purposes = query.data?.data ?? [];
  return (
    <Panel
      title={t("settings.purposes")}
      titleAction={
        canAdminister ? (
          <Button aria-haspopup="dialog" onClick={() => setAdding(true)}>
            {t("privacy.addPurpose")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <PanelIntro>{t("settings.purposesSub")}</PanelIntro>
        {me.isSuccess && !canAdminister && (
          <p className="t-caption">{t("privacy.purposesReadOnly")}</p>
        )}
        {(!query.isSuccess || purposes.length === 0) && (
          <QueryStates
            query={query}
            pendingLabel={t("privacy.purposesLoading")}
          >
            <EmptyState>{t("privacy.purposesEmpty")}</EmptyState>
          </QueryStates>
        )}
      </PanelBody>
      <CardBoundary>
        {query.isSuccess && purposes.length > 0 && (
          <PurposeTable purposes={purposes} />
        )}
      </CardBoundary>
      <Modal
        open={adding}
        onClose={() => setAdding(false)}
        labelledBy={addTitleId}
        intent="form"
      >
        <Heading size="large" id={addTitleId} className="t-h2 modal-title">
          {t("privacy.addPurpose")}
        </Heading>
        <PurposeCreateForm onDone={() => setAdding(false)} />
      </Modal>
    </Panel>
  );
}

function PurposeTable({ purposes }: Readonly<{ purposes: ConsentPurpose[] }>) {
  const t = useT();
  const columns: DataTableColumn<ConsentPurpose>[] = [
    {
      key: "purpose",
      header: t("privacy.purpose"),
      render: (purpose) => (
        <CellStack>
          <span>{purpose.label}</span>
          <span className="t-caption privacy-wrap">{purpose.key}</span>
        </CellStack>
      ),
    },
    {
      key: "doi",
      header: t("privacy.purposeDoiColumn"),
      fold: "end",
      render: (purpose) =>
        purpose.requires_double_opt_in ? (
          <Badge tone="info">{t("privacy.purposeDoiBadge")}</Badge>
        ) : null,
    },
  ];
  return (
    <DataTable
      label={t("privacy.purposesRegistry")}
      bleed
      fold
      columns={columns}
      rows={purposes}
      rowKey={(purpose) => purpose.id}
    />
  );
}
