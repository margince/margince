// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DataTable } from "../design-system/datatable";
import { FileDropzone } from "../design-system/filedropzone";
import { Select } from "../design-system/select";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { ImportContextTag } from "./import.contexttag";
import type { ImportWizardState, WizardFile } from "./importflow";
import type { ImportFormat, ImportObject, ImportProfile } from "./importtypes";
import { isImportObject } from "./importtypes";

/** The drop zone the hub and the wizard's first step share. */
export function ImportDropzone({
  sizeLimitMb,
  onPick,
}: Readonly<{ sizeLimitMb?: number; onPick: (file: File) => void }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <FileDropzone
      label={t("import.hub.uploadLabel")}
      hint={
        sizeLimitMb === undefined
          ? undefined
          : t("import.hub.uploadHint", {
              mb: formatNumber(sizeLimitMb, locale),
            })
      }
      emptyLabel={t("import.hub.uploadEmpty")}
      accept=".zip,.csv"
      onPick={onPick}
    />
  );
}

type Clash = Readonly<{ first: string; second: string; object: ImportObject }>;

// The first two files the reader gave one type: a run takes one file per type.
function firstClash(
  files: readonly WizardFile[],
  objects: Readonly<Record<string, ImportObject | null>>,
): Clash | null {
  const seen = new Map<ImportObject, string>();
  for (const { file } of files) {
    const object = objects[file];
    if (!object) continue;
    const first = seen.get(object);
    if (first !== undefined) return { first, second: file, object };
    seen.set(object, file);
  }
  return null;
}

/**
 * The files of an upload, each with its type, plus the run's source name and
 * the tag for created records. Next is refused,
 * with the reason beside it, while no file has a type or two share one.
 */
export function FilesStep({
  wizard,
  format,
}: Readonly<{ wizard: ImportWizardState; format: ImportFormat }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const typed = wizard.files.filter((file) => wizard.objects[file.file]);
  const clash = firstClash(wizard.files, wizard.objects);
  let refusal: string | undefined;
  if (typed.length === 0) {
    refusal = t("import.files.noneTyped");
  } else if (clash) {
    refusal = t("import.files.clash", {
      first: clash.first,
      second: clash.second,
      type: t(`import.object.${clash.object}`),
    });
  }
  const types = [
    { value: "", label: t("import.files.skip") },
    ...format.order.map((object) => ({
      value: object,
      label: t(`import.object.${object}`),
    })),
  ];
  return (
    <div className="import">
      <DataTable<WizardFile>
        label={t("import.files.table")}
        rows={wizard.files}
        rowKey={(file) => file.file}
        columns={[
          {
            key: "file",
            header: t("import.files.col.file"),
            grow: true,
            render: (file) => (
              <>
                <code>{file.file}</code>
                <div className="t-caption">
                  {plural("import.files.columns", file.columns.length, {
                    count: formatNumber(file.columns.length, locale),
                  })}
                </div>
              </>
            ),
          },
          {
            key: "rows",
            header: t("import.files.col.rows"),
            align: "end",
            render: (file) => formatNumber(file.rowsProfiled, locale),
          },
          {
            key: "type",
            header: t("import.files.col.type"),
            render: (file) => (
              <Select
                options={types}
                value={wizard.objects[file.file] ?? ""}
                disabled={wizard.upload.isPending}
                onChange={(next) =>
                  wizard.setFileObject(
                    file.file,
                    isImportObject(next) ? next : null,
                  )
                }
                aria-label={t("import.files.typeFor", { file: file.file })}
              />
            ),
          },
        ]}
      />
      <IgnoredFiles ignored={wizard.profile?.ignored ?? []} />
      <Field
        label={t("import.files.sourceName")}
        hint={t("import.files.sourceNameHint")}
      >
        {(control) => (
          <TextInput
            {...control}
            value={wizard.sourceName}
            onChange={(event) => wizard.setSourceName(event.target.value)}
          />
        )}
      </Field>
      <ImportContextTag
        value={wizard.contextTagID}
        onChange={wizard.setContextTagID}
      />
      <div className="import__actions">
        <Button variant="ghost" onClick={wizard.reset}>
          {t("import.wizard.cancel")}
        </Button>
        <Button
          variant="primary"
          reason={refusal}
          pending={wizard.upload.isPending}
          busyLabel={t("import.upload.reading")}
          onClick={() => wizard.goTo("map")}
        >
          {plural("import.files.next", typed.length, {
            count: formatNumber(typed.length, locale),
          })}
        </Button>
      </div>
    </div>
  );
}

// The ZIP entries no file was made from, each with why, so a file left out is
// never a silent one.
function IgnoredFiles({
  ignored,
}: Readonly<{ ignored: NonNullable<ImportProfile["ignored"]> }>) {
  const t = useT();
  if (ignored.length === 0) return null;
  return (
    <Callout kind="standing" title={t("import.files.ignoredTitle")}>
      <ul className="import__issues t-sub">
        {ignored.map((entry) => (
          <li key={entry.file}>
            {entry.file}: {t(`import.files.ignored.${entry.reason}`)}
          </li>
        ))}
      </ul>
    </Callout>
  );
}
