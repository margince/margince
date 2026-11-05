// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useId, useRef } from "react";
import { useUnsavedGuard } from "../../components/unsaved";
import { AppLink } from "../../design-system/applink";
import { useArrivalFocus } from "../../design-system/arrivalfocus";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryStates } from "../../design-system/querystates";
import { RecordBack } from "../../design-system/recordview";
import { StageLadder } from "../../design-system/stageladder";
import { useLocale, useT } from "../../i18n";
import { routeHash } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { FilesStep, ImportDropzone } from "./import.files";
import { ImportRefusal } from "./import.notices";
import { ImportPresetTarget } from "./import.preset";
import { ImportProgress, ImportResult } from "./import.result";
import { TestRunStep } from "./import.testrun";
import { runHref } from "./importaddress";
import {
  takeHandedOverUpload,
  uploadObjectFor,
  type WizardStep,
} from "./importflow";
import { MapStep } from "./importmapping";
import { useImportFormat, useRecentImports, wroteNothing } from "./importrun";
import type { ImportFormat, ImportRun } from "./importtypes";
import { type ImportWizardState, useImportWizard } from "./importwizard";

const STEPS: readonly WizardStep[] = [
  "upload",
  "files",
  "map",
  "test",
  "import",
  "result",
];

function stagedHint(wizard: ImportWizardState) {
  return wizard.step === "result"
    ? "import.step.hintDone"
    : "import.step.hintStaged";
}

/** The run flow's page: it owns the flow's state and draws the flow. */
export function ImportWizardPage() {
  const wizard = useImportWizard();
  const { locale } = useLocale();
  const format = useImportFormat(locale);
  const recent = useRecentImports();
  // Only the upload still on its way is work the page could lose. Once it
  // answers, the draft is the server's and the hub lists it.
  useUnsavedGuard(wizard.upload.isPending);
  // The file the hub's drop zone took starts here, once the format can say
  // which type a CSV's name gives. The page holds it, so a page closed before
  // then drops it rather than uploading it on a later visit.
  const { upload } = wizard;
  const presetObject = wizard.presetContact ? "activity" : undefined;
  const handedOver = useRef<File | null>(null);
  useEffect(() => {
    handedOver.current ??= takeHandedOverUpload();
  }, []);
  useEffect(() => {
    if (!format.data) return;
    const file = handedOver.current;
    handedOver.current = null;
    if (file) {
      upload.mutate({
        file,
        object: uploadObjectFor(file.name, format.data, presetObject),
        recent: recent.data,
        retype: false,
      });
    }
  }, [format.data, recent.data, upload, presetObject]);
  return <ImportWizard wizard={wizard} />;
}

/**
 * The steps as one ladder, and the step the reader is on. An earlier step
 * opens again only while the run is a draft whose upload this page holds.
 */
export function ImportWizard({
  wizard,
}: Readonly<{ wizard: ImportWizardState }>) {
  const t = useT();
  const { locale } = useLocale();
  const format = useImportFormat(locale);
  const title = useArrivalFocus<HTMLSpanElement>();
  const hintId = useId();
  const here = STEPS.indexOf(wizard.step);
  return (
    <div className="import-wizard">
      <RecordBack
        href={routeHash(settingsHref("import"))}
        label={t("import.back")}
      />
      <StageLadder
        wrap
        label={t("import.step.label")}
        hint={
          <span id={hintId}>
            {wizard.editable ? t("import.step.hint") : t(stagedHint(wizard))}
          </span>
        }
        steps={STEPS.map((step, index) => {
          const back =
            index < here &&
            wizard.editable &&
            (step === "files" || step === "map");
          return {
            key: step,
            label: t(`import.step.${step}`),
            current: index === here,
            done: index < here,
            onPick: () => {
              if (step === "files" || step === "map") wizard.goTo(step);
            },
            reasonId: back ? undefined : hintId,
          };
        })}
      />
      {wizard.presetContact ? (
        <ImportPresetTarget contactId={wizard.presetContact} />
      ) : null}
      {wizard.conflict ? (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("import.conflictOtherTab")}
          actions={
            <Button variant="ghost" onClick={wizard.dismissConflict}>
              {t("import.conflictDismiss")}
            </Button>
          }
        />
      ) : null}
      <Panel
        title={
          <span ref={title} tabIndex={-1}>
            {t(`import.step.title.${wizard.step}`)}
          </span>
        }
        titleAction={
          wizard.profile?.file_name ? (
            <span className="t-caption">{wizard.profile.file_name}</span>
          ) : undefined
        }
      >
        <PanelBody>
          <QueryStates query={format} pendingLabel={t("common.loading")}>
            {format.data ? (
              <StepBody wizard={wizard} format={format.data} />
            ) : null}
          </QueryStates>
        </PanelBody>
      </Panel>
    </div>
  );
}

function StepBody({
  wizard,
  format,
}: Readonly<{ wizard: ImportWizardState; format: ImportFormat }>) {
  const t = useT();
  if (wizard.editable) {
    switch (wizard.step) {
      case "files":
        return <FilesStep wizard={wizard} format={format} />;
      case "map":
        return <MapStep wizard={wizard} format={format} />;
      default:
        return <UploadStep wizard={wizard} format={format} />;
    }
  }
  const run = wizard.run.data;
  return (
    <QueryStates query={wizard.run} pendingLabel={t("common.loading")}>
      {run && wroteNothing(run.status) ? (
        <ClosedRun wizard={wizard} run={run} />
      ) : null}
      {run && wizard.step === "test" ? (
        <TestRunStep wizard={wizard} format={format} />
      ) : null}
      {run && wizard.step === "import" ? (
        <ImportProgress wizard={wizard} format={format} run={run} />
      ) : null}
      {run && wizard.step === "result" ? (
        <ImportResult wizard={wizard} format={format} run={run} />
      ) : null}
    </QueryStates>
  );
}

// A draft whose file this page no longer holds cannot go on here. Nor can a
// cancelled run. The reader is told why and starts from an upload.
function ClosedRun({
  wizard,
  run,
}: Readonly<{ wizard: ImportWizardState; run: ImportRun }>) {
  const t = useT();
  const cancelled = run.status === "cancelled";
  const newer = run.superseded_by;
  const title = newer
    ? t("import.superseded")
    : t(cancelled ? "import.closed.cancelled" : "import.closed.draft");
  return (
    <Callout
      tone={cancelled ? "info" : "warning"}
      kind="standing"
      title={title}
      actions={
        newer ? (
          <AppLink className="link-button" href={runHref(newer)}>
            {t("import.supersededOpen")}
          </AppLink>
        ) : (
          <Button variant="primary" onClick={wizard.reset}>
            {t("import.closed.upload")}
          </Button>
        )
      }
    >
      {cancelled ? undefined : (run.error ?? t("import.closed.stopped"))}
    </Callout>
  );
}

function UploadStep({
  wizard,
  format,
}: Readonly<{ wizard: ImportWizardState; format: ImportFormat }>) {
  const t = useT();
  const recent = useRecentImports();
  const { upload } = wizard;
  return (
    <div className="import">
      <ImportDropzone
        sizeLimitMb={format.size_limit_mb}
        csvOnly={wizard.presetContact !== null}
        onPick={(file) =>
          upload.mutate({
            file,
            object: uploadObjectFor(
              file.name,
              format,
              wizard.presetContact ? "activity" : undefined,
            ),
            recent: recent.data,
            retype: false,
          })
        }
      />
      {upload.isPending ? (
        <p className="import__hint t-sub" aria-live="polite">
          {t("import.upload.reading")}
        </p>
      ) : null}
      {upload.error ? (
        <ImportRefusal title={t("import.uploadFailed")} error={upload.error} />
      ) : null}
    </div>
  );
}
