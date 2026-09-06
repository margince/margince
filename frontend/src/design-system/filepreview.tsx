// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Download, Printer, X } from "lucide-react";
import type { ReactNode, RefObject } from "react";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useT } from "../i18n";
import { EmptyState, Modal, PendingBody } from "./atoms";
import { IconAction } from "./iconaction";
import "./filepreview.css";

/**
 * One stored file, opened over the page it was clicked on.
 *
 * `FileChip` is the card; this is what the card opens. A reader who clicks a
 * contract wants to READ it, and the answer the product had was a download —
 * which puts the paper in another application, behind whatever that machine
 * has installed, with the record it belongs to no longer on screen. So the
 * file opens here instead, on the darkened page, with the three verbs a reader
 * of a document actually has: save it, print it, put it away.
 *
 * The bytes are fetched ONCE and shown from a blob, which is what makes this
 * possible at all: the byte endpoints answer `Content-Disposition: attachment`,
 * so a frame pointed straight at one downloads the file rather than drawing it.
 * That header is not a nuisance to be removed — it is what stops an uploaded
 * file from being served as a document under this origin — so the preview
 * works around it on OUR side, and re-types the bytes from the allowlist in
 * `filechip.tsx` rather than from anything the response said.
 *
 * A PDF goes in a frame and an image in an `<img>`, and the split is not a
 * preference: a browser fits an image to the window only when the image IS the
 * document, so a photo inside a frame is drawn at its own size and a scan of an
 * A4 page becomes a scrolling wall of paper two inches from the reader's face.
 * A PDF, conversely, is drawn by a viewer this page cannot style or reach past,
 * and it is the better viewer — pages, zoom, a search box.
 *
 * That split is also why PRINT has two spellings, in `printing` below: a frame
 * prints itself, and an image is drawn by this page, so the page prints and the
 * `@media print` rules leave the document as the only thing on the sheet.
 */
export type PreviewFile = Readonly<{
  /** Same-origin path to the bytes — the same one `FileChip` links to. */
  href: string;
  filename: string;
  /**
   * What the bytes are DRAWN as. Decided by the caller from the filename
   * (`previewMediaType` in `filechip.tsx`) and never read off the response,
   * because a blob URL inherits this origin: a type that can carry script
   * would carry it into our own page with our own cookies.
   */
  mediaType: string;
}>;

type FilePreview = Readonly<{ open: (file: PreviewFile) => void }>;

const FilePreviewContext = createContext<FilePreview | null>(null);

/**
 * The one preview, claimed from wherever a file is drawn.
 *
 * Null rather than a no-op default, and unlike `useToast` that is deliberate:
 * the caller is a LINK, and it has to know whether pressing it opens something
 * here before it cancels the navigation it would otherwise perform. A story or
 * a unit test that renders a `FileChip` on its own has no provider, and the
 * card there stays exactly what it was — a download.
 */
export function useFilePreview(): FilePreview | null {
  return useContext(FilePreviewContext);
}

/**
 * Where the preview lives: once, above the screens.
 *
 * One mount for the reason `OpenEmailDrawer` is one mount per record — a
 * dialog per surface is two `aria-modal` boxes and two Escape handlers the
 * first time two of them are open together — and held to this file by
 * `conformance.test.ts`.
 */
export function FilePreviewProvider({
  children,
}: Readonly<{ children: ReactNode }>) {
  const [file, setFile] = useState<PreviewFile | null>(null);
  const open = useCallback((opened: PreviewFile) => setFile(opened), []);
  const close = useCallback(() => setFile(null), []);
  const controls = useMemo<FilePreview>(() => ({ open }), [open]);
  return (
    <FilePreviewContext.Provider value={controls}>
      {children}
      <FilePreviewDialog file={file} onClose={close} />
    </FilePreviewContext.Provider>
  );
}

// The heading is the filename, and the dialog is named by it. One id because
// there is one dialog: a second would be the defect the single mount exists to
// prevent, and it would announce under this one's name.
const TITLE_ID = "file-preview-name";

/**
 * Mounted whether or not a file is open, so `Modal` keeps the keyboard
 * contract it owns: focus goes into the dialog when it opens and back to the
 * chip that opened it when it closes, and a dialog conditionally rendered
 * around that is a dialog that unmounts before it can put focus back.
 */
function FilePreviewDialog({
  file,
  onClose,
}: Readonly<{ file: PreviewFile | null; onClose: () => void }>) {
  const t = useT();
  const frame = useRef<HTMLIFrameElement | null>(null);
  const object = usePreviewObject(file);
  return (
    <Modal
      open={file !== null}
      onClose={onClose}
      placement="full"
      labelledBy={TITLE_ID}
    >
      {file !== null && (
        <div className="file-preview">
          <div className="file-preview-head">
            <h2 id={TITLE_ID} className="t-h3 file-preview-name">
              {file.filename}
            </h2>
            <div className="file-preview-verbs">
              <IconAction
                small
                label={t("filePreview.download")}
                icon={<Download size={15} aria-hidden="true" />}
                onClick={() => save(file)}
              />
              {/* Print stands beside the other two only once there IS a
                  document: until the bytes arrive, and after a read that
                  failed, there is nothing on the stage for it to put on paper.
                  Absent rather than refused, because `Button`'s refusal
                  contract prints its sentence beside the control — right for a
                  row of worded verbs, and in a row of three glyphs it is a
                  paragraph hanging off the corner of the dialog. */}
              {object.status === "ready" && (
                <IconAction
                  small
                  label={t("filePreview.print")}
                  icon={<Printer size={15} aria-hidden="true" />}
                  onClick={() => printing(frame.current)}
                />
              )}
              <IconAction
                small
                label={t("filePreview.close")}
                icon={<X size={15} aria-hidden="true" />}
                onClick={onClose}
              />
            </div>
          </div>
          <div className="file-preview-stage">
            <PreviewBody file={file} object={object} frame={frame} />
          </div>
        </div>
      )}
    </Modal>
  );
}

/** What the stage holds: the document, or what is standing in for it. */
function PreviewBody({
  file,
  object,
  frame,
}: Readonly<{
  file: PreviewFile;
  object: PreviewObject;
  frame: RefObject<HTMLIFrameElement | null>;
}>) {
  const t = useT();
  if (object.status === "loading") {
    return <PendingBody label={t("filePreview.loading")} lines={4} />;
  }
  if (object.status === "failed") {
    return (
      <EmptyState title={t("filePreview.failedTitle")}>
        {t("filePreview.failed")}
      </EmptyState>
    );
  }
  if (file.mediaType === PDF) {
    return (
      <iframe
        ref={frame}
        className="file-preview-frame"
        src={object.url}
        // The frame's accessible name, and the only one it can have: what is
        // inside it is a document the browser drew, not markup this page can
        // label.
        title={file.filename}
      />
    );
  }
  // The filename as the alternative text, because it is the only description
  // anything here has. A scan's subject is in the pixels and nowhere in the
  // record, so the honest line is what the file is CALLED — which is also what
  // the card the reader pressed said.
  return (
    <img className="file-preview-image" src={object.url} alt={file.filename} />
  );
}

// The one kind drawn by a viewer rather than by this page. Named because two
// places ask the same question — which element draws it, and which print does.
const PDF = "application/pdf";

/**
 * Printing what is on the stage, in the only way each kind allows.
 *
 * A frame holds a document the browser drew, and the page cannot reach into it
 * to lay it out — so it is asked to print ITSELF, which is what puts the PDF's
 * own pagination on the paper. An image is drawn by this page, so the page is
 * what prints, and the `@media print` rules in `filepreview.css` and
 * `atoms.css` take the application behind the scrim off the sheet.
 *
 * Two spellings rather than one because there is no third thing both can do:
 * a page-level print of a frame holding a PDF prints an empty box, and a frame
 * around an image is the layout this component exists to avoid.
 */
function printing(frame: HTMLIFrameElement | null) {
  const viewer = frame?.contentWindow;
  if (viewer !== null && viewer !== undefined) {
    viewer.print();
    return;
  }
  window.print();
}

/**
 * Saving the file, through a link the page never draws.
 *
 * The three header verbs are one row of glyphs and read as one control each;
 * the odd one out being an `<a>` styled to look like the two buttons beside it
 * is how a tree grows a second icon button. The act is still the anchor's —
 * `download` on the same href the chip carries — so the saved copy is the one
 * the server names, whether or not the preview above ever loaded.
 */
function save(file: PreviewFile) {
  const link = document.createElement("a");
  link.href = file.href;
  link.download = file.filename;
  // In the document before the click: Safari ignores `download` on a node that
  // is not in one, and saves nothing at all.
  document.body.append(link);
  link.click();
  link.remove();
}

type PreviewObject =
  | Readonly<{ status: "loading" }>
  | Readonly<{ status: "ready"; url: string }>
  | Readonly<{ status: "failed" }>;

/**
 * The opened file's bytes, as an object URL this page owns.
 *
 * Revoked on the way out, every time. An object URL is held by the document
 * until it is released, so a reader who opens six scans in a session would
 * otherwise be carrying all six for as long as the tab lives.
 */
function usePreviewObject(file: PreviewFile | null): PreviewObject {
  const [object, setObject] = useState<PreviewObject>({ status: "loading" });
  useEffect(() => {
    if (file === null) {
      return;
    }
    setObject({ status: "loading" });
    const abort = new AbortController();
    let url: string | null = null;
    fetch(file.href, { credentials: "include", signal: abort.signal })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`preview read answered ${response.status}`);
        }
        const bytes = await response.arrayBuffer();
        url = URL.createObjectURL(new Blob([bytes], { type: file.mediaType }));
        setObject({ status: "ready", url });
      })
      .catch(() => {
        // WHY the read failed is not drawn. A refusal, a cut connection and a
        // file the store no longer holds leave the reader the same one move —
        // save it and open it elsewhere — and the internals of the read are
        // not theirs to be shown. An abort is this effect's own cleanup and
        // not a failure at all.
        if (!abort.signal.aborted) {
          setObject({ status: "failed" });
        }
      });
    return () => {
      abort.abort();
      if (url !== null) {
        URL.revokeObjectURL(url);
      }
    };
  }, [file]);
  return object;
}
