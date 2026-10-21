import { Field } from "../../design-system/atoms";
import { FileDropzoneControl } from "../../design-system/filedropzone";
import { SurfaceState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import { ACCEPTED_ATTACHMENT_ATTR } from "../../lib/attachmentupload";
import { paperState, useContractPaper } from "./contractpaper";

/**
 * SignedFileField shows the paper already on file and takes a new one by
 * drag-and-drop or by clicking.
 *
 * It lists what is already there. A form that only offers an upload tells its
 * reader that nothing is filed yet. Someone checking an agreement's terms wants
 * the signed PDF, and clicking the row lands them on this edit form. A filed
 * document reachable only from somewhere else reads as one that does not exist.
 *
 * It never claims an absence it does not know. A read in flight, a failed
 * read and a refused grant all leave the answer unknown. A bare drop zone in
 * any of them would make the same false claim this field exists to stop.
 * Reading documents needs its own grant, so a reader without it is told they
 * are withheld, not shown an empty field.
 *
 * It never presents a page as a list. The documents endpoint paginates, so an
 * agreement with more paper than one page holds is `partial` and says how much
 * is missing.
 *
 * The picker takes both gestures. Dropping suits a reader with a PDF in front
 * of them, and clicking works from a keyboard and on a phone. A drop zone with
 * no real input behind it is unreachable without a mouse, so the input stays
 * present and is made invisible.
 */
export function SignedFileField({
  companyId,
  contractID,
  file,
  onPick,
}: Readonly<{
  companyId: string;
  contractID?: string;
  file?: File;
  onPick: (file: File) => void;
}>) {
  const t = useT();
  const filed = useContractPaper(companyId, contractID);

  const paper = filed.data;
  const onFile = paper?.documents ?? [];
  const state = paperState(Boolean(contractID), filed, paper);
  // A drop zone always shows, since uploading does not depend on reading what
  // is filed. Hiding the only way to attach paper would punish the reader for
  // a grant they lack. What changes is whether anything above it
  // claims the full picture. `empty` is the one state that does, and the only
  // one whose picker says "drop a file here".

  return (
    <Field label={t("contracts.form.file")} hint={t("contracts.form.fileHint")}>
      {(props) => (
        <>
          {/* Each filed document, downloadable by name, or the reason when
              the answer is not known. `empty` renders nothing here
              because the drop zone below already says the field is waiting for
              a file. Two sentences saying the same absence is noise.

              `ready` and `partial` draw the same links; `partial` adds what is
              missing under them, which is where a count about a list belongs. */}
          {state === "empty" ? null : (
            <SurfaceState
              loadingLabel={t("contracts.form.file")}
              state={state}
              emptyLabel=""
              detail={{
                onRetry: () => void filed.refetch(),
                remaining: paper?.remaining,
              }}
            >
              {onFile.map((doc) => (
                // `.link-button`, not the row link: a row title is a link because
                // of where it sits. In a form a plain-coloured line reads as a
                // typed value, and this one has to look like a download.
                <a
                  key={doc.id}
                  className="link-button"
                  href={`/v1/attachments/${doc.id}`}
                  download={doc.filename}
                >
                  {doc.title || doc.filename}
                </a>
              ))}
            </SurfaceState>
          )}
          {/* The zone is the design-system control, and this field owns the
              `Field` around it so the filed paper above can sit under the same
              label. It uses `fileAdd` whenever this field does not assert that
              nothing is filed. That holds when paper is filed (an agreement can
              carry an amendment beside its original). It also holds when the
              read did not come back, where "drop a file here" would restate the
              absence the panel above declined to claim. */}
          <FileDropzoneControl
            control={props}
            file={file}
            onPick={onPick}
            accept={ACCEPTED_ATTACHMENT_ATTR}
            emptyLabel={t(
              state !== "empty"
                ? "contracts.form.fileAdd"
                : "contracts.form.fileEmpty",
            )}
          />
        </>
      )}
    </Field>
  );
}
