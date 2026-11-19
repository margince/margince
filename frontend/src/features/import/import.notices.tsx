// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { EntityRef } from "../../components/entityref";
import { AppLink } from "../../design-system/applink";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { formatNumber } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { Locale } from "../../i18n/locale";
import { recordEnabled, useHiddenCoreFeatures } from "../../lib/corefeatures";
import { isEntityKind } from "../../lib/entity";
import { valueFor } from "../../lib/labelfor";
import {
  type FieldProblem,
  ProblemError,
  problemCode,
  problemFieldErrorsOf,
  problemMessageOf,
} from "../../lib/problem";
import { isRecord } from "../../lib/problembody";
import { runHref } from "./importaddress";
import { fieldLabelOf } from "./importfieldlabel";
import { reasonText } from "./importreason";
import {
  type ImportFormat,
  type ImportObject,
  type ImportReport,
  isImportObject,
} from "./importtypes";

// What an UNDO says about itself: that it stopped part-way, and which rows it
// could not put back.
//
// Their own file because both are bands whose body is a LIST or a caveat about
// the run rather than about the file, and the wizard's own function already
// carries the upload, the mapping, the dry run, the commit and the reversal.

type UndoOutcome = NonNullable<ImportReport["undo"]>;

/** A record an undo left in place, by its name as a link. A record the reader
 * cannot see, or one with no page, shows its id. */
export function UndoRecordName({
  object,
  id,
}: Readonly<{ object: ImportObject; id: string }>) {
  // EntityRef renders nothing for a kind this reader cannot read, and the
  // undo outcome must still say which record it kept.
  const hidden = useHiddenCoreFeatures();
  return isEntityKind(object) && recordEnabled(object, hidden) ? (
    <EntityRef kind={object} id={id} />
  ) : (
    id
  );
}

/**
 * The rows a reversal could not put back, named with why — under ONE heading,
 * as the notice's own body.
 *
 * The lead used to be a band standing OVER the list it introduced, which is a
 * heading in notice clothing: the rows are what the reader came here to read,
 * and a callout whose body is the list says the same thing with one anatomy.
 */
export function UndoErrors({
  rows,
}: Readonly<{ rows: UndoOutcome["errored"] }>) {
  const t = useT();
  if (rows.length === 0) {
    return null;
  }
  return (
    <Callout tone="warning" kind="outcome" title={t("import.undoErroredLead")}>
      <ul className="import__issues t-sub">
        {rows.map((row) => (
          <li key={`${row.object}-${row.id}`}>
            {t(`import.object.${row.object}`)} —{" "}
            <UndoRecordName object={row.object} id={row.id} />:{" "}
            {reasonText(t, row.reason, row.reason_code, undefined)}
          </li>
        ))}
      </ul>
    </Callout>
  );
}

/**
 * What an undo will do, asked before it starts. `order` is the import order
 * from the format; the undo walks it backwards, so the list is reversed here.
 */
export function UndoConfirm({
  open,
  order,
  pending,
  error,
  onClose,
  onConfirm,
}: Readonly<{
  open: boolean;
  order: readonly ImportObject[];
  pending: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: () => void;
}>) {
  const t = useT();
  const reversed = [...order]
    .reverse()
    .map((object) => t(`import.object.${object}`));
  return (
    <ConfirmModal
      open={open}
      title={t("import.undoConfirm.title")}
      confirmLabel={t("import.undoConfirm.confirm")}
      confirmVariant="danger"
      pending={pending}
      error={error}
      onClose={onClose}
      onConfirm={onConfirm}
    >
      {reversed.length > 0 ? (
        <p>{t("import.undoConfirm.order", { order: reversed.join(", ") })}</p>
      ) : null}
      <ul className="import__issues">
        <li>{t("import.undoConfirm.archived")}</li>
        <li>{t("import.undoConfirm.kept")}</li>
        <li>{t("import.undoConfirm.updated")}</li>
        <li>{t("import.undoConfirm.tags")}</li>
      </ul>
    </ConfirmModal>
  );
}

// The run that holds a busy source name, when the reader may read it.
function busyRunOf(error: unknown): string | null {
  if (!(error instanceof ProblemError)) return null;
  const { problem } = error;
  if (problemCode(problem) !== "import_source_busy" || !isRecord(problem)) {
    return null;
  }
  const runId = isRecord(problem.details) ? problem.details.run_id : null;
  return typeof runId === "string" ? runId : null;
}

// One sentence of a refusal: for one `reason` of its code when it names one,
// and only once every value it puts in has arrived.
type Wording = Readonly<{
  key: MessageKey;
  reason?: string;
  needs?: readonly string[];
}>;

type Refusal = Readonly<{
  wordings: readonly Wording[];
  // A refusal of one file in a ZIP names that file before its sentence.
  inFile?: boolean;
}>;

// Keyed by field and code. Each list ends in a wording that needs nothing, so
// a known code always has words of its own and never the server's.
const REFUSALS: Readonly<Record<string, Refusal>> = {
  "file/not_utf8": {
    inFile: true,
    wordings: [{ key: "import.refusal.notUtf8" }],
  },
  "file/unreadable": {
    inFile: true,
    wordings: [
      {
        key: "import.refusal.unreadable.fieldCount",
        reason: "field_count",
        needs: ["line"],
      },
      {
        key: "import.refusal.unreadable.quote",
        reason: "quote",
        needs: ["line"],
      },
      { key: "import.refusal.unreadable.line", needs: ["line"] },
      { key: "import.refusal.unreadable" },
    ],
  },
  "file/header_unusable": {
    inFile: true,
    wordings: [
      {
        key: "import.refusal.header.repeatedName",
        reason: "repeated_name",
        needs: ["name"],
      },
      {
        key: "import.refusal.header.blankName",
        reason: "blank_name",
        needs: ["position"],
      },
      { key: "import.refusal.header.noHeader", reason: "no_header" },
      { key: "import.refusal.header" },
    ],
  },
  "file/bundle_refused": {
    wordings: [
      {
        key: "import.refusal.bundle.twoFilesOneType",
        reason: "two_files_one_type",
        needs: ["file", "other_file", "object"],
      },
      {
        key: "import.refusal.bundle.tooManyFiles",
        reason: "too_many_files",
        needs: ["files", "max_files"],
      },
      { key: "import.refusal.bundle.empty", reason: "empty" },
      { key: "import.refusal.bundle.spreadsheet", reason: "spreadsheet" },
      {
        key: "import.refusal.bundle.noImportableFile",
        reason: "no_importable_file",
        needs: ["file_names"],
      },
      { key: "import.refusal.bundle.notAZip", reason: "not_a_zip" },
      {
        key: "import.refusal.bundle.filesTooLarge",
        reason: "files_too_large",
        needs: ["max_mb"],
      },
      {
        key: "import.refusal.bundle.zipTooLarge",
        reason: "zip_too_large",
        needs: ["max_mb"],
      },
      {
        key: "import.refusal.bundle.passwordProtected",
        reason: "password_protected",
        needs: ["file"],
      },
      {
        key: "import.refusal.bundle.fileDamaged",
        reason: "file_damaged",
        needs: ["file"],
      },
      {
        key: "import.refusal.bundle.fileMissing",
        reason: "file_missing",
        needs: ["file"],
      },
      {
        key: "import.refusal.bundle.typeNotImportable",
        reason: "type_not_importable",
        needs: ["object"],
      },
      { key: "import.refusal.bundle" },
    ],
  },
  "file/empty": {
    wordings: [{ key: "import.refusal.fileEmpty" }],
  },
  "file/too_many_rows": {
    wordings: [
      { key: "import.refusal.tooManyRows", needs: ["rows", "max"] },
      { key: "import.upload.tooManyRows" },
    ],
  },
  "object/unsupported": {
    wordings: [{ key: "import.refusal.unsupported" }],
  },
  "object/mismatch": {
    wordings: [{ key: "import.refusal.objectMismatch" }],
  },
  "mapping/empty": {
    wordings: [{ key: "import.refusal.mappingEmpty" }],
  },
  "mapping/unknown_column": {
    wordings: [{ key: "import.refusal.unknownColumn" }],
  },
  "mapping/unknown_target": {
    wordings: [{ key: "import.refusal.unknownTarget" }],
  },
  "mapping/duplicate_target": {
    wordings: [
      {
        key: "import.refusal.duplicateTarget.named",
        needs: ["target", "column", "other_column"],
      },
      { key: "import.refusal.duplicateTarget" },
    ],
  },
  "source_key/unmapped_column": {
    wordings: [{ key: "import.refusal.unmappedColumn" }],
  },
  "source_key/unmappable": {
    wordings: [
      {
        key: "import.refusal.unmappable.targetOrRef",
        needs: ["target", "ref_target"],
      },
      { key: "import.refusal.unmappable.target", needs: ["target"] },
      { key: "import.refusal.unmappable" },
    ],
  },
  "source_name/invalid": {
    wordings: [
      { key: "import.refusal.sourceName", needs: ["max"] },
      { key: "import.files.sourceNameInvalid" },
    ],
  },
};

const FIGURES = new Set([
  "line",
  "position",
  "files",
  "max_files",
  "max_mb",
  "rows",
]);

type RefusalWords = Readonly<{
  t: Translator;
  locale: Locale;
  format: ImportFormat | undefined;
}>;

// What the Map step calls a field, so a refusal names the field the reader
// chose there. A file with no format around it keeps the field's key.
function fieldLabel(
  target: string,
  object: string | undefined,
  format: ImportFormat | undefined,
): string {
  const column = (format?.files ?? [])
    .filter((file) => object === undefined || file.object === object)
    .flatMap((file) => file.columns)
    .find((one) => one.name === target);
  return column ? fieldLabelOf(column) : target;
}

// The server sends every value as text, a figure among them.
function wordedValue(
  name: string,
  value: string,
  values: Readonly<Record<string, string>>,
  { t, locale, format }: RefusalWords,
): string {
  if (FIGURES.has(name) && /^\d+$/.test(value)) {
    return formatNumber(Number(value), locale);
  }
  if (name === "object" && isImportObject(value)) {
    return t(`import.object.${value}`);
  }
  if (name === "target" || name === "ref_target") {
    return fieldLabel(value, values.object, format);
  }
  return value;
}

/**
 * The reader's sentence for one field refusal, or null for a code this build
 * does not know, which keeps the server's own sentence.
 */
export function refusalSentence(
  fault: FieldProblem,
  words: RefusalWords,
): string | null {
  const refusal = valueFor(REFUSALS, `${fault.field}/${fault.code}`, null);
  if (refusal === null) return null;
  const values: Record<string, string> = {
    ...fault.params,
    ...(fault.max === undefined
      ? null
      : { max: formatNumber(fault.max, words.locale) }),
  };
  const wording = refusal.wordings.find(
    (one) =>
      (one.reason === undefined || one.reason === values.reason) &&
      (one.needs ?? []).every((name) => values[name] !== undefined),
  );
  if (wording === undefined) return null;
  const sentence = words.t(
    wording.key,
    Object.fromEntries(
      Object.entries(values).map(([name, value]) => [
        name,
        wordedValue(name, value, values, words),
      ]),
    ),
  );
  return refusal.inFile && values.file !== undefined
    ? words.t("import.refusal.inFile", { file: values.file, sentence })
    : sentence;
}

/** Why the server refused an upload or a test run, in the reader's words. */
export function ImportRefusal({
  title,
  error,
  format,
  keepsDraft = false,
}: Readonly<{
  title: string;
  error: unknown;
  /** Names a refused field by its label on the Map step. */
  format?: ImportFormat;
  /** The refused call was a stage, which leaves the upload as a draft. Undo,
   * cancel and approve leave none, so the draft sentence is theirs to omit. */
  keepsDraft?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const faults = problemFieldErrorsOf(error);
  const tooLong = faults.some((field) => field.code === "too_many_rows");
  const sentence = faults
    .map((fault) => refusalSentence(fault, { t, locale, format }))
    .find((one) => one !== null);
  const busyRun = busyRunOf(error);
  return (
    <Callout
      tone="danger"
      kind="outcome"
      title={tooLong ? t("import.upload.tooManyRowsTitle") : title}
      actions={
        busyRun ? (
          <AppLink className="link-button" href={runHref(busyRun)}>
            {t("import.sourceBusyOpen")}
          </AppLink>
        ) : undefined
      }
    >
      {sentence ?? problemMessageOf(error, t)}
      {busyRun && keepsDraft ? <p>{t("import.sourceBusyDraft")}</p> : null}
    </Callout>
  );
}
