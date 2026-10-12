// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { routeHash } from "../app/router";
import { useArrivalFocus } from "../design-system/arrivalfocus";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { RecordBack } from "../design-system/recordview";
import { StageLadder } from "../design-system/stageladder";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, QueryStates } from "./common";
import { FilesStep, ImportDropzone } from "./import.files";
import {
  type ImportWizardState,
  uploadObjectFor,
  useImportFormat,
  useImportWizard,
  useRecentImports,
  type WizardStep,
} from "./importflow";
import { MapStep } from "./importmapping";
import type { ImportFormat } from "./importtypes";
import { settingsHref } from "./settingsrouting";

const STEPS: readonly WizardStep[] = [
  "upload",
  "files",
  "map",
  "test",
  "import",
  "result",
];

/** The run flow's page: it owns the flow's state and draws the flow. */
export function ImportWizardPage() {
  const wizard = useImportWizard();
  return <ImportWizard wizard={wizard} />;
}

/**
 * The steps as one ladder, and the step the reader is on. A step behind them
 * opens again only while no run exists, because a staged run was made from
 * the files and the map as they were.
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
        label={t("import.step.label")}
        hint={<span id={hintId}>{t("import.step.hint")}</span>}
        steps={STEPS.map((step, index) => {
          const back =
            index < here &&
            wizard.runId === null &&
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
  switch (wizard.step) {
    case "upload":
      return <UploadStep wizard={wizard} format={format} />;
    case "files":
      return <FilesStep wizard={wizard} format={format} />;
    case "map":
      return <MapStep wizard={wizard} format={format} />;
    default:
      return null;
  }
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
        onPick={(file) =>
          upload.mutate({
            file,
            object: uploadObjectFor(file.name, format),
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
        <Callout tone="danger" kind="outcome" title={t("import.uploadFailed")}>
          {problemMessageOf(upload.error, t)}
        </Callout>
      ) : null}
    </div>
  );
}
