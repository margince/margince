// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useCan, useCanWrite } from "../app/capability";
import { navigate, routeHash } from "../app/router";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { QueryStates } from "../design-system/querystates";
import { RecordBack } from "../design-system/recordview";
import { formatDate, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { useMemberNames } from "../lib/membernames";
import { problemMessageOf } from "../lib/problem";
import { useMe } from "../lib/session";
import { ImportDropzone } from "./import.files";
import { UndoConfirm } from "./import.notices";
import { IMPORT_TEMPLATE_HREF, TemplatePreview } from "./import.template";
import { ImportWizardPage } from "./import.wizard";
import { runHref } from "./importaddress";
import { handOverUpload } from "./importflow";
import {
  useImportFormat,
  useLandedRecords,
  useRecentImports,
  useUndoImport,
  wroteNothing,
} from "./importrun";
import type { ImportFormat, ImportRun } from "./importtypes";
import {
  IMPORT_RUN_SUBPAGE,
  IMPORT_TEMPLATE_SUBPAGE,
  settingsHref,
} from "./settingsrouting";
import "./import.css";

// Bringing a customer's data into the estate. Download the template or upload
// a file on the hub, then check, map, test and import on the run page.
//
// An import is the least reversible write in the product: thousands of rows
// across several record types in one act. So nothing is written until a human
// has read the dry run's report and pressed the button again.
const IMPORT_HOME = settingsHref("import");
const openRun = () => navigate({ ...IMPORT_HOME, id2: IMPORT_RUN_SUBPAGE });
const openTemplate = () =>
  navigate({ ...IMPORT_HOME, id2: IMPORT_TEMPLATE_SUBPAGE });

export function ImportCard({ subpage }: Readonly<{ subpage?: string }>) {
  const t = useT();
  // The flow does not only create: the dry run parks the run and the approval
  // moves it (update). A role edited to create-without-update would see the card
  // and be refused at the first button. useCanWrite folds the seat ceiling, which
  // a read-seat admin would otherwise hit as a clamped POST.
  const mayCreate = useCanWrite("import_run", "create");
  const mayAdvance = useCan("import_run", "update");
  const mayImport = mayCreate && mayAdvance;
  const me = useMe();

  // Gated on the grant the STORE demands, not on the admin role: `import_run`
  // is seeded to admin AND ops, so asking for the role would hide the card
  // from an ops seat the server would have accepted.
  //
  // Withheld rather than absent, and gated on the PROBE rather than its absence.
  // Maintenance opens on `isAdmin || embedding_reindex:read`, so a seat holding
  // only the reindex grant reaches this page — and a card that simply is not
  // there tells them this installation cannot import, which is a claim about the
  // product rather than about their authority. Every capability hook also fails
  // closed while /me is in flight, so branching before it answers would flash
  // the notice at the admin who holds the grant.
  if (me.isSuccess && !mayImport) {
    return (
      <Panel title={t("import.title")}>
        <PanelBody>
          <EmptyState>{t("import.withheld")}</EmptyState>
        </PanelBody>
      </Panel>
    );
  }
  if (!mayImport) {
    return null;
  }
  if (subpage === IMPORT_RUN_SUBPAGE) {
    return <ImportWizardPage />;
  }
  if (subpage === IMPORT_TEMPLATE_SUBPAGE) {
    return (
      <div>
        <RecordBack href={routeHash(IMPORT_HOME)} label={t("import.back")} />
        <TemplatePreview />
      </div>
    );
  }
  return <ImportHub />;
}

// The hub shows the two ways in (a template to fill, or a file to upload).
// It also shows the order files land in, and the runs already made.
function ImportHub() {
  const t = useT();
  const { locale } = useLocale();
  const format = useImportFormat(locale);
  // The file goes to the run page, which starts the upload. The drop zone
  // unmounts with the hub, and the run page's title takes the focus.
  const start = (file: File) => {
    handOverUpload(file);
    openRun();
  };
  return (
    <>
      <Panel title={t("import.hub.title")}>
        <PanelBody>
          <div className="import-hub__ways">
            <div className="import-hub__way">
              <Heading size="small" as="h3">
                {t("import.hub.templateTitle")}
              </Heading>
              <p className="import__hint t-sub">
                {t("import.hub.templateBody")}
              </p>
              <div className="import-hub__verbs">
                <Button variant="ghost" onClick={openTemplate}>
                  {t("import.hub.preview")}
                </Button>
                <a className="link-button" href={IMPORT_TEMPLATE_HREF} download>
                  {t("import.template.download")}
                </a>
              </div>
            </div>
            <ImportDropzone
              sizeLimitMb={format.data?.size_limit_mb}
              onPick={start}
            />
          </div>
          {format.data ? <ImportOrder order={format.data.order} /> : null}
        </PanelBody>
      </Panel>
      <RecentImports format={format.data} />
    </>
  );
}

// The order the engine lands files in, read from the format. A link can only
// name a record an earlier file made, so a reader planning a ZIP needs it.
function ImportOrder({ order }: Readonly<{ order: ImportFormat["order"] }>) {
  const t = useT();
  return (
    <p className="import-hub__order t-sub">
      <span>{t("import.hub.order")}</span>
      {order.map((object, index) => (
        <span key={object} className="import-hub__step">
          {index > 0 ? <span aria-hidden>→</span> : null}
          <Badge>{t(`import.object.${object}`)}</Badge>
        </span>
      ))}
    </p>
  );
}

const STATUS_TONE = {
  pending: "default",
  validating: "info",
  awaiting_approval: "default",
  running: "info",
  complete: "success",
  failed: "danger",
  undoing: "info",
  undone: "default",
  cancelled: "default",
} as const satisfies Record<ImportRun["status"], string>;

// The rows a run holds, once a dry run has counted them.
function rowsOf(run: ImportRun): number {
  return (run.parts ?? []).reduce((sum, part) => sum + part.row_count, 0);
}

// The runs made so far, newest first. Undo is offered on a complete run only:
// every other state either is still moving or has nothing left to reverse.
function RecentImports({ format }: Readonly<{ format?: ImportFormat }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const recent = useRecentImports();
  const runs = (recent.data ?? []).filter((one) => !wroteNothing(one.status));
  useLandedRecords(runs.map((one) => one.status));
  const names = useMemberNames(runs.map((one) => one.captured_by));
  const undo = useUndoImport();
  const [undoing, setUndoing] = useState<string | null>(null);
  const close = () => {
    setUndoing(null);
    undo.reset();
  };
  return (
    <Panel title={t("import.recent.title")}>
      <PanelBody>
        <QueryStates query={recent} pendingLabel={t("common.loading")}>
          {runs.length === 0 ? (
            <EmptyState>{t("import.recent.empty")}</EmptyState>
          ) : (
            <DataTable<ImportRun>
              label={t("import.recent.title")}
              rows={runs}
              rowKey={(one) => one.id}
              columns={[
                {
                  key: "date",
                  header: t("import.recent.col.date"),
                  render: (one) =>
                    formatDate(one.created_at, locale, viewerZone()),
                },
                {
                  key: "file",
                  header: t("import.recent.col.file"),
                  grow: true,
                  render: (one) => {
                    const files = plural(
                      "import.recent.files",
                      one.parts?.length ?? 0,
                      { count: formatNumber(one.parts?.length ?? 0, locale) },
                    );
                    const by = names.get(one.captured_by);
                    return (
                      <>
                        <a className="cell-link" href={runHref(one.id)}>
                          {one.file_name || one.source_name || one.id}
                        </a>
                        <div className="t-caption">
                          {by
                            ? t("import.recent.filesBy", { files, name: by })
                            : files}
                        </div>
                      </>
                    );
                  },
                },
                {
                  key: "rows",
                  header: t("import.recent.col.rows"),
                  align: "end",
                  render: (one) => formatNumber(rowsOf(one), locale),
                },
                {
                  key: "status",
                  header: t("import.recent.col.status"),
                  render: (one) => (
                    <Badge tone={STATUS_TONE[one.status]}>
                      {t(`import.status.${one.status}`)}
                    </Badge>
                  ),
                },
                {
                  key: "actions",
                  header: t("import.recent.col.actions"),
                  headerHidden: true,
                  render: (one) =>
                    one.status === "complete" ? (
                      <div className="cell-actions">
                        <Button
                          variant="ghost"
                          onClick={() => setUndoing(one.id)}
                        >
                          {t("import.recent.undo")}
                        </Button>
                      </div>
                    ) : null,
                },
              ]}
            />
          )}
        </QueryStates>
      </PanelBody>
      <UndoConfirm
        open={undoing !== null}
        order={format?.order ?? []}
        pending={undo.isPending}
        error={undo.error ? problemMessageOf(undo.error, t) : null}
        onClose={close}
        onConfirm={() => {
          if (undoing !== null) undo.mutate(undoing, { onSuccess: close });
        }}
      />
    </Panel>
  );
}
