import { File, FileText, Image, type LucideIcon } from "lucide-react";
import type { MouseEvent } from "react";
import { useFilePreview } from "./filepreview";
import "./filechip.css";

// A stored file, drawn as the small card a reader clicks to get it.
//
// Distinct from `Chip`, which is a FACT about a record and links off our
// origin: this is the file itself, so it is a same-origin link and its name is
// the filename the file was uploaded under — the name the saved copy will
// have. A row that carries paper should say WHICH paper, and two files on one
// row have to be tellable apart before the click, not after it.
//
// The card is an anchor and stays one, but a file the browser can DRAW opens
// in `FilePreview` instead of leaving for the downloads folder: a reader who
// clicks a contract on a record is reading, and the record is still what they
// are in the middle of. Everything else — a .docx, a spreadsheet — has no
// reading here to offer and keeps the download unchanged.
//
// The glyph and the kind tag both name the kind and nothing more: they are
// decorative, and the filename between them is the accessible name. Two kinds
// get a glyph of their own — PDF, which is what agreement paper arrives as,
// and an image, which is what a photo, a scan or a mail signature's logo
// arrives as — because those are the two a reader most wants to tell apart
// from each other and from everything else before the click.
export function FileChip({
  href,
  filename,
  size,
}: Readonly<{
  // Same-origin path to the bytes. This control is for OUR files; anything
  // pointing off the origin is a link, not a file.
  href: string;
  filename: string;
  // Already formatted for the reader's locale. A size is what tells a 400 KB
  // scan from a 40 MB one before the click; a file whose size was never
  // recorded simply says nothing rather than "0".
  size?: string;
}>) {
  const preview = useFilePreview();
  const kind = fileKind(filename);
  const { glyph: Glyph, preview: mediaType } = factsFor(kind);
  return (
    <a
      className="file-chip"
      href={href}
      download={filename}
      onClick={(event) => {
        // Three ways this stays a download, and each is a reader getting what
        // they asked for: nothing here can draw the file, no preview is
        // mounted (a story, a unit test), or a modifier says they want a new
        // tab or a saved copy rather than a dialog over the page.
        if (preview === null || mediaType === null || opensElsewhere(event)) {
          return;
        }
        event.preventDefault();
        preview.open({ href, filename, mediaType });
      }}
    >
      <Glyph size={14} aria-hidden="true" />
      {kind && (
        <span className="file-chip-kind" aria-hidden="true">
          {kind}
        </span>
      )}
      <span className="file-chip-name">{filename}</span>
      {size && <span className="file-chip-size">{size}</span>}
    </a>
  );
}

// Whether the reader asked for this file somewhere OTHER than here: a new tab,
// a new window, a saved copy. Cancelling the navigation under any of those
// takes away a move the browser offers on every link in the product.
function opensElsewhere(event: MouseEvent<HTMLAnchorElement>): boolean {
  return event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
}

// What a filename says about the file: the mark the card draws, and the media
// type the preview may draw it AS — null for a kind that can only be saved.
//
// ONE table rather than a glyph list beside a preview list, because both are
// answered from the same key and the rows where they DISAGREE are the ones
// worth seeing: a picture a reader recognises is not always a picture a
// browser can open, and the two-list spelling put that fact in two places for
// the next author to keep in step by hand.
//
// The kinds are read from the extension rather than guessed from the bytes:
// that is what a reader will see in their own downloads folder, and a card
// disagreeing with it would be lying about the file it points at.
type FileKind = Readonly<{ glyph: LucideIcon; preview: string | null }>;

const KINDS: ReadonlyMap<string, FileKind> = new Map([
  // Agreement paper, and the pictures a phone camera, a scanner or a mail
  // signature writes. These are the kinds a reader most wants to tell apart
  // before the click, and the only ones with a mark of their own.
  ["PDF", { glyph: FileText, preview: "application/pdf" }],
  ["JPG", { glyph: Image, preview: "image/jpeg" }],
  ["JPEG", { glyph: Image, preview: "image/jpeg" }],
  ["PNG", { glyph: Image, preview: "image/png" }],
  ["GIF", { glyph: Image, preview: "image/gif" }],
  ["WEBP", { glyph: Image, preview: "image/webp" }],
  ["BMP", { glyph: Image, preview: "image/bmp" }],
  // Pictures to a reader and to no browser: they keep the mark and lose the
  // preview, because a stage drawing nothing is worse than a plain download.
  ["HEIC", { glyph: Image, preview: null }],
  ["TIFF", { glyph: Image, preview: null }],
  ["TIF", { glyph: Image, preview: null }],
  // A picture's extension over a document that can carry script. The preview
  // draws from a blob under THIS origin, so an SVG opened there would run as
  // our own page with our own cookies — it is never drawn, whatever it is.
  ["SVG", { glyph: Image, preview: null }],
]);

// A kind this product does not recognise gets the neutral mark rather than a
// guessed one, and no preview: what the browser would make of it is not
// something a filename can promise.
const UNKNOWN_KIND: FileKind = { glyph: File, preview: null };

function factsFor(kind: string): FileKind {
  return KINDS.get(kind) ?? UNKNOWN_KIND;
}

/**
 * The media type the preview may draw this file as, or null for one that can
 * only be saved.
 *
 * Answered from the FILENAME and from the table above, never from the
 * response's own content type: the bytes are shown from a blob this page
 * created, and a blob URL inherits this origin, so the one thing that must not
 * decide how they are interpreted is the store they came out of.
 */
export function previewMediaType(filename: string): string | null {
  return factsFor(fileKind(filename)).preview;
}

// The extension, upper-cased, or "" for a filename that carries none. Capped
// at four characters so a dotted name without one ("Q3.final report") is read
// as having no extension rather than as a kind nobody has heard of.
function fileKind(filename: string): string {
  const dot = filename.lastIndexOf(".");
  const extension = dot > 0 ? filename.slice(dot + 1) : "";
  return extension.length > 0 && extension.length <= 4
    ? extension.toUpperCase()
    : "";
}
