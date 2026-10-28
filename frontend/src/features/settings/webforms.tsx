// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Globe } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import { ifMatch } from "../../api/version";
import { ArchiveAction } from "../../components/archive";
import { Badge, Button, OverflowMenu } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { WebFormDrawer } from "./webformdrawer";
import {
  mintsSecret,
  pastesSecret,
  WEB_FORMS_GUIDE,
  WEB_FORMS_KEY,
  type WebForm,
  type WebFormCreated,
} from "./webformkinds";
import { FormOwnerLine } from "./webformowner";
import { WebFormSetupModal } from "./webformsetup";
import "./webforms.css";

// Settings → Website forms: the website forms that turn a visitor's enquiry
// into a lead. A form is an intake address; the setup dialog says how a
// website, its server or a form builder posts to it.

// The form to show the setup for, and the secret a create or rotate minted.
// The secret lives only here, never in the query cache.
type Setup = Readonly<{ form: WebForm; secret?: string }>;

function useWebForms() {
  return useQuery({
    queryKey: [WEB_FORMS_KEY],
    queryFn: async () => {
      const { data, error } = await api.GET("/web-forms", {
        params: { query: {} },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

async function archiveWebForm(form: WebForm): Promise<WebForm> {
  const data = unwrap(
    await api.DELETE("/web-forms/{id}", {
      params: { path: { id: form.id } },
    }),
  );
  return data ?? form;
}

async function setFormState(form: WebForm): Promise<WebForm> {
  return unwrap(
    await api.PATCH("/web-forms/{id}", {
      params: { path: { id: form.id }, ...ifMatch(form.version) },
      body: { state: form.state === "active" ? "paused" : "active" },
    }),
  );
}

async function rotateFormSecret(form: WebForm): Promise<WebFormCreated> {
  return unwrap(
    await api.POST("/web-forms/{id}/rotate-secret", {
      params: { path: { id: form.id } },
    }),
  );
}

function PauseAction({ form }: Readonly<{ form: WebForm }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const toggle = useMutation({
    mutationFn: setFormState,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: [WEB_FORMS_KEY] }),
  });
  return (
    <>
      <Button
        data-testid="toggle-web-form"
        pending={toggle.isPending}
        onClick={() => toggle.mutate(form)}
      >
        {form.state === "active" ? t("webForms.pause") : t("webForms.resume")}
      </Button>
      {toggle.isError && (
        <ErrorLine>{problemMessageOf(toggle.error, t)}</ErrorLine>
      )}
    </>
  );
}

function RotateAction({
  form,
  onRotated,
}: Readonly<{ form: WebForm; onRotated: (setup: Setup) => void }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const rotate = useMutation({
    mutationFn: rotateFormSecret,
    onSuccess: (rotated) => {
      queryClient.invalidateQueries({ queryKey: [WEB_FORMS_KEY] });
      setConfirming(false);
      onRotated({ form: rotated, secret: rotated.signing_secret });
    },
  });
  return (
    <>
      <Button
        variant="danger"
        data-testid="rotate-web-form-secret"
        onClick={() => setConfirming(true)}
      >
        {t("webForms.rotate")}
      </Button>
      <ConfirmModal
        open={confirming}
        onClose={() => {
          setConfirming(false);
          rotate.reset();
        }}
        title={t("webForms.rotateConfirm.title")}
        confirmLabel={t("webForms.rotate")}
        confirmVariant="danger"
        onConfirm={() => rotate.mutate(form)}
        pending={rotate.isPending}
        error={rotate.isError ? problemMessageOf(rotate.error, t) : null}
      >
        <p className="t-body">{t("webForms.rotateConfirm.body")}</p>
      </ConfirmModal>
    </>
  );
}

function FormRow({
  form,
  canEdit,
  canArchive,
  onSetup,
}: Readonly<{
  form: WebForm;
  canEdit: boolean;
  canArchive: boolean;
  onSetup: (setup: Setup) => void;
}>) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  return (
    <>
      <SettingRow
        label={form.name}
        description={
          <span className="webform-row-description">
            {form.sender === "browser"
              ? form.allowed_origins.join(", ")
              : t(`webForms.senderLine.${form.sender}`)}
            {form.owner_id && <FormOwnerLine ownerId={form.owner_id} />}
            <span className="webform-row-actions">
              <Button
                data-testid="web-form-setup"
                onClick={() => onSetup({ form })}
              >
                {t("webForms.setup")}
              </Button>
              {canEdit && (
                <Button
                  data-testid="edit-web-form"
                  onClick={() => setEditing(true)}
                >
                  {t("webForms.edit")}
                </Button>
              )}
              {(canEdit || canArchive) && (
                <OverflowMenu label={t("record.moreActions")}>
                  {canEdit && <PauseAction form={form} />}
                  {canEdit && mintsSecret(form.sender) && (
                    <RotateAction form={form} onRotated={onSetup} />
                  )}
                  {canArchive && (
                    <ArchiveAction
                      requires={{ object: "integrations", action: "delete" }}
                      label={t("webForms.archive")}
                      confirmText={t("webForms.archiveConfirm")}
                      archivedMessage={t("webForms.archiveDone")}
                      invalidate={WEB_FORMS_KEY}
                      recordKey="web-form"
                      onArchived={() => {}}
                      archive={() => archiveWebForm(form)}
                    />
                  )}
                </OverflowMenu>
              )}
            </span>
          </span>
        }
        value={
          <span className="webform-answer">
            <Badge tone={form.state === "active" ? "success" : "warning"}>
              {t(`webForms.state.${form.state}`)}
            </Badge>
            <Badge tone="default">{t(`webForms.sender.${form.sender}`)}</Badge>
            {form.consent_required && (
              <Badge tone="default">{t("webForms.row.consent")}</Badge>
            )}
            {!form.secret_configured && pastesSecret(form.sender) && (
              <Badge tone="warning">{t("webForms.row.secretMissing")}</Badge>
            )}
          </span>
        }
        control={null}
      />
      {editing && (
        <WebFormDrawer form={form} onClose={() => setEditing(false)} />
      )}
    </>
  );
}

export function WebFormsCard() {
  const t = useT();
  const canCreate = useCanWrite("integrations", "create");
  const canEdit = useCanWrite("integrations", "update");
  const canArchive = useCanWrite("integrations", "delete");
  const me = useMe();
  const query = useWebForms();
  const [creating, setCreating] = useState(false);
  const [setup, setSetup] = useState<Setup | null>(null);
  const showReadOnly = me.isSuccess && !canCreate && !canEdit && !canArchive;
  return (
    <Panel
      title={t("webForms.title")}
      titleAction={
        canCreate && query.isSuccess ? (
          <Button
            variant="primary"
            data-testid="new-web-form"
            onClick={() => setCreating(true)}
          >
            <Globe aria-hidden /> {t("webForms.new")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <PanelIntro>
          {t("webForms.sub")}{" "}
          <a href={WEB_FORMS_GUIDE} target="_blank" rel="noreferrer">
            {t("webForms.guide")}
          </a>
        </PanelIntro>
        {showReadOnly && <PanelIntro>{t("webForms.readOnly")}</PanelIntro>}
        <QueryGate
          pendingLabel={t("webForms.title")}
          query={query}
          empty={(result) => result.data.length === 0}
        >
          {(result) => (
            <SettingList>
              {result.data.map((form) => (
                <FormRow
                  key={form.id}
                  form={form}
                  canEdit={canEdit}
                  canArchive={canArchive}
                  onSetup={setSetup}
                />
              ))}
            </SettingList>
          )}
        </QueryGate>
      </PanelBody>
      {creating && query.data && (
        <WebFormDrawer
          secretsAvailable={query.data.secrets_available}
          onClose={() => setCreating(false)}
          onCreated={(created) =>
            setSetup({ form: created, secret: created.signing_secret })
          }
        />
      )}
      {setup && (
        <WebFormSetupModal
          form={setup.form}
          secret={setup.secret}
          onClose={() => setSetup(null)}
        />
      )}
    </Panel>
  );
}
