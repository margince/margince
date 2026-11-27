// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Globe } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import { ifMatch } from "../../api/version";
import { ArchiveAction } from "../../components/archive";
import { PageHeadAction } from "../../components/pageheadaction";
import { WebsiteInquiryDialog } from "../../components/websiteinquirydialog";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryStates } from "../../design-system/querystates";
import {
  formatDate,
  formatDateTime,
  formatTimeOfDay,
} from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { useViewerZone } from "../../lib/viewerzone";
import { WEBSITE_INQUIRY_RECIPE } from "../../lib/websiteinquiries";
import { WebFormDrawer } from "./webformdrawer";
import {
  mintsSecret,
  pastesSecret,
  WEB_FORMS_GUIDE,
  WEB_FORMS_KEY,
  type WebForm,
  type WebFormCreated,
} from "./webformkinds";
import { FormOwnerCell } from "./webformowner";
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

function HandlingAction({
  form,
  canEdit,
}: Readonly<{ form: WebForm; canEdit: boolean }>) {
  const t = useT();
  const [handling, setHandling] = useState(false);
  const readPolicies = useCan("automation", "read");
  const createPolicies = useCanWrite("automation", "create");
  const editPolicies = useCanWrite("automation", "update");
  const canHandle =
    canEdit &&
    readPolicies &&
    editPolicies &&
    (!!form.handling_automation_id || createPolicies);
  const catalog = useQuery({
    queryKey: ["automation-catalog"],
    enabled: canHandle,
    queryFn: async () => unwrap(await api.GET("/automations/catalog")),
  });
  if (!canHandle) return null;
  if (catalog.error) return <ErrorLine inline error={catalog.error} />;
  if (!catalog.data?.data.some((entry) => entry.key === WEBSITE_INQUIRY_RECIPE))
    return null;
  return (
    <>
      <Button onClick={() => setHandling(true)}>{t("inquiry.handling")}</Button>
      {handling && (
        <WebsiteInquiryDialog form={form} onClose={() => setHandling(false)} />
      )}
    </>
  );
}

// Every verb a form offers, behind one menu so the row reads as facts. A press
// on the row itself opens Edit, the verb an admin reaches for most.
function FormMenu({
  form,
  canEdit,
  canArchive,
  onSetup,
  onEdit,
}: Readonly<{
  form: WebForm;
  canEdit: boolean;
  canArchive: boolean;
  onSetup: (setup: Setup) => void;
  onEdit: (form: WebForm) => void;
}>) {
  const t = useT();
  return (
    <OverflowMenu label={t("table.rowActions", { name: form.name })}>
      <Button data-testid="web-form-setup" onClick={() => onSetup({ form })}>
        {t("webForms.setup")}
      </Button>
      {canEdit && (
        <Button data-testid="edit-web-form" onClick={() => onEdit(form)}>
          {t("webForms.edit")}
        </Button>
      )}
      <HandlingAction form={form} canEdit={canEdit} />
      {canEdit && <PauseAction form={form} />}
      {canEdit && mintsSecret(form.sender) && (
        <RotateAction form={form} onRotated={onSetup} />
      )}
      {canArchive && (
        <ArchiveAction
          requires={{ object: "integrations", action: "delete" }}
          label={t("webForms.archive")}
          title={t("webForms.archiveTitle", { name: form.name })}
          confirmText={t("webForms.archiveConfirm")}
          archivedMessage={t("webForms.archiveDone")}
          invalidate={WEB_FORMS_KEY}
          recordKey="web-form"
          onArchived={() => {}}
          archive={() => archiveWebForm(form)}
        />
      )}
    </OverflowMenu>
  );
}

// A browser form names the websites it takes posts from under its sender; the
// signed senders need no address, so the sender alone says it.
function SenderCell({ form }: Readonly<{ form: WebForm }>) {
  const t = useT();
  return (
    <CellStack>
      <span>{t(`webForms.sender.${form.sender}`)}</span>
      {form.sender === "browser" && form.allowed_origins.length > 0 && (
        <span className="t-caption">{form.allowed_origins.join(", ")}</span>
      )}
    </CellStack>
  );
}

function StateCell({ form }: Readonly<{ form: WebForm }>) {
  const t = useT();
  return (
    <span className="webform-answer">
      <Badge tone={form.state === "active" ? "success" : "warning"} dot>
        {t(`webForms.state.${form.state}`)}
      </Badge>
      {!form.secret_configured && pastesSecret(form.sender) && (
        <Badge tone="warning">{t("webForms.row.secretMissing")}</Badge>
      )}
      {form.consent_required && (
        <Badge tone="default">{t("webForms.row.consent")}</Badge>
      )}
    </span>
  );
}

function formColumns({
  t,
  canEdit,
  canArchive,
  onSetup,
  onEdit,
}: Readonly<{
  t: ReturnType<typeof useT>;
  canEdit: boolean;
  canArchive: boolean;
  onSetup: (setup: Setup) => void;
  onEdit: (form: WebForm) => void;
}>): DataTableColumn<WebForm>[] {
  return [
    {
      key: "name",
      header: t("webForms.col.form"),
      grow: true,
      fold: "title",
      render: (form) => form.name,
    },
    {
      key: "sender",
      header: t("webForms.col.sender"),
      render: (form) => <SenderCell form={form} />,
    },
    {
      key: "owner",
      header: t("webForms.col.owner"),
      render: (form) => (
        <FormOwnerCell
          ownerId={form.owner_id}
          handled={!!form.handling_automation_id}
        />
      ),
    },
    {
      key: "lastPost",
      header: t("webForms.col.lastPost"),
      render: (form) => <LastPostCell form={form} />,
    },
    {
      key: "state",
      header: t("webForms.col.state"),
      render: (form) => <StateCell form={form} />,
    },
    {
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (form) => (
        <span className="cell-actions">
          <FormMenu
            form={form}
            canEdit={canEdit}
            canArchive={canArchive}
            onSetup={onSetup}
            onEdit={onEdit}
          />
        </span>
      ),
    },
  ];
}

// Whether the pasted code works, read off the last post the form landed. An
// older server sends no field at all, so the cell says nothing either way.
function LastPostCell({ form }: Readonly<{ form: WebForm }>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const lastPostAt = form.last_post_at;
  return (
    <CellStack>
      {lastPostAt === null && (
        <span className="webform-none">{t("webForms.row.noPost")}</span>
      )}
      {lastPostAt && (
        <span className="t-num">
          {formatDate(lastPostAt, locale, readerZone)}
        </span>
      )}
      <IntakeLine intake={form.intake} />
    </CellStack>
  );
}

// The throttle's refusals, so an admin whose site sees 429s knows why.
function IntakeLine({ intake }: Readonly<{ intake: WebForm["intake"] }>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  if (!intake) {
    return null;
  }
  if (intake.state === "throttled" && intake.since) {
    return (
      <span data-testid="web-form-intake">
        {t("webForms.row.intake.throttled", {
          when: formatTimeOfDay(intake.since, locale, readerZone),
        })}
      </span>
    );
  }
  if (intake.state === "blocked" && intake.until) {
    return (
      <span data-testid="web-form-intake">
        {t("webForms.row.intake.blocked", {
          when: formatDateTime(intake.until, locale, readerZone),
        })}
      </span>
    );
  }
  if (intake.state === "paused_all") {
    return (
      <span data-testid="web-form-intake">
        {t("webForms.row.intake.pausedAll")}
      </span>
    );
  }
  return null;
}

export function WebFormsCard() {
  const t = useT();
  const canCreate = useCanWrite("integrations", "create");
  const canEdit = useCanWrite("integrations", "update");
  const canArchive = useCanWrite("integrations", "delete");
  const me = useMe();
  const query = useWebForms();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<WebForm | null>(null);
  const [setup, setSetup] = useState<Setup | null>(null);
  const showReadOnly = me.isSuccess && !canCreate && !canEdit && !canArchive;
  const forms = query.data?.data ?? [];
  const columns = formColumns({
    t,
    canEdit,
    canArchive,
    onSetup: setSetup,
    onEdit: setEditing,
  });
  return (
    <>
      {canCreate && query.isSuccess && (
        <PageHeadAction>
          <Button
            variant="primary"
            data-testid="new-web-form"
            onClick={() => setCreating(true)}
          >
            <Globe aria-hidden /> {t("webForms.new")}
          </Button>
        </PageHeadAction>
      )}
      {/* Untitled: on a one-panel page the page heading names the forms. */}
      <Panel>
        <PanelBody>
          <PanelIntro>
            <a href={WEB_FORMS_GUIDE} target="_blank" rel="noreferrer">
              {t("webForms.guide")}
            </a>
          </PanelIntro>
          {showReadOnly && <PanelIntro>{t("webForms.readOnly")}</PanelIntro>}
          {!(query.isSuccess && forms.length > 0) && (
            <QueryStates query={query} pendingLabel={t("webForms.title")}>
              <EmptyState>{t("common.empty")}</EmptyState>
            </QueryStates>
          )}
        </PanelBody>
        {query.isSuccess && forms.length > 0 && (
          <DataTable
            bleed
            fold
            label={t("webForms.title")}
            columns={columns}
            rows={[...forms]}
            rowKey={(form) => form.id}
            rowTestId={(form) => `web-form-${form.id}`}
            onRowClick={canEdit ? setEditing : undefined}
          />
        )}
      </Panel>
      {creating && query.data && (
        <WebFormDrawer
          secretsAvailable={query.data.secrets_available}
          onClose={() => setCreating(false)}
          onCreated={(created) =>
            setSetup({ form: created, secret: created.signing_secret })
          }
        />
      )}
      {editing && (
        <WebFormDrawer form={editing} onClose={() => setEditing(null)} />
      )}
      {setup && (
        <WebFormSetupModal
          form={setup.form}
          secret={setup.secret}
          onClose={() => setSetup(null)}
        />
      )}
    </>
  );
}
