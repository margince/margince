import pdfWorkerUrl from "pdfjs-dist/legacy/build/pdf.worker.min.mjs?url";

// What a file handed to the voice corpus is read AS, spelled once for both
// surfaces that collect writing samples (the onboarding voice act and the
// Settings Voice DNA card). The server takes text only — it has no honest word
// count for a binary document — so a PDF or a Word file becomes text HERE, in
// the browser, and travels the same wire a .txt does. The accept list every
// picker and every by-name check uses is derived from this table, so a format
// cannot be offered without a reader, or read without being offered.

type CorpusFileReader = (file: File) => Promise<string>;

const plainText: CorpusFileReader = (file) => file.text();

const corpusFileReaders: Readonly<Record<string, CorpusFileReader>> = {
  txt: plainText,
  md: plainText,
  vtt: plainText,
  srt: plainText,
  json: plainText,
  pdf: async (file) => pdfText(await file.arrayBuffer()),
  docx: async (file) => docxText(await file.arrayBuffer()),
};

/** The `accept` attribute for a corpus file picker. */
export const ACCEPTED_CORPUS_ATTR = Object.keys(corpusFileReaders)
  .map((extension) => `.${extension}`)
  .join(",");

function readerFor(name: string): CorpusFileReader | null {
  const dot = name.lastIndexOf(".");
  if (dot === -1) {
    return null;
  }
  return corpusFileReaders[name.slice(dot + 1).toLowerCase()] ?? null;
}

/** Whether a filename is one the corpus accepts at all. */
export function isAcceptedCorpusFile(name: string): boolean {
  return readerFor(name) !== null;
}

/**
 * The text a corpus file carries, or null when it cannot be read as one: a
 * name with no reader, a PDF that is encrypted or damaged, a .docx that is not
 * a Word document. A readable file that holds no words (a scanned PDF) is the
 * empty string, which the intake's own empty gate names for the reader.
 */
export async function readCorpusFile(file: File): Promise<string | null> {
  const read = readerFor(file.name);
  if (read === null) {
    return null;
  }
  try {
    return await read(file);
  } catch (err) {
    // The parser's own message names bytes and offsets, never anything a
    // reader could act on; the surface says the file could not be opened.
    console.warn("voice corpus file could not be read", file.name, err);
    return null;
  }
}

// PDF: pdf.js, loaded only when a PDF actually arrives — it is the one heavy
// dependency on this path and most corpora never need it. The legacy build is
// the one that runs both in a browser and under Node (the test runtime); under
// Node pdf.js runs its worker in-process and has already chosen a worker
// module of its own, so the bundled worker asset is set only where nothing is.
async function pdfText(bytes: ArrayBuffer): Promise<string> {
  const pdfjs = await import("pdfjs-dist/legacy/build/pdf.mjs");
  pdfjs.GlobalWorkerOptions.workerSrc ||= pdfWorkerUrl;
  const document = await pdfjs.getDocument({ data: new Uint8Array(bytes) })
    .promise;
  try {
    const pages: string[] = [];
    for (let number = 1; number <= document.numPages; number++) {
      const page = await document.getPage(number);
      const content = await page.getTextContent();
      pages.push(
        content.items
          .map((item) =>
            "str" in item ? `${item.str}${item.hasEOL ? "\n" : ""}` : "",
          )
          .join(""),
      );
    }
    return pages.join("\n\n");
  } finally {
    await document.destroy();
  }
}

// DOCX: a zip archive whose `word/document.xml` carries the body. Only that
// one part is read, straight from the central directory, and only the text
// runs are kept — tracked deletions (`w:delText`) and field codes
// (`w:instrText`) are other elements and fall away with the markup.
const WORD_BODY_PART = "word/document.xml";

async function docxText(bytes: ArrayBuffer): Promise<string> {
  const body = await zipEntry(bytes, WORD_BODY_PART);
  if (body === null) {
    throw new Error(`no ${WORD_BODY_PART} part`);
  }
  return wordBodyText(new TextDecoder().decode(body));
}

// The WordprocessingML elements that become characters. Longest name first so
// `w:tab` is never read as `w:t` followed by junk; every other element —
// `w:pPr`, `w:r`, `w:delText` — matches nothing here and contributes nothing.
const WORD_TEXT_TAG = /<(\/?)(w:tab|w:t|w:p|w:br|w:cr)(?:\s[^>]*?)?(\/?)>/g;

function wordBodyText(xml: string): string {
  const out: string[] = [];
  let textStart = -1;
  for (const tag of xml.matchAll(WORD_TEXT_TAG)) {
    const [whole, closing, name, selfClosing] = tag;
    if (name !== "w:t") {
      out.push(breakFor(name, closing === "/"));
      continue;
    }
    if (closing === "/" && textStart !== -1) {
      out.push(decodeXmlEntities(xml.slice(textStart, tag.index)));
    }
    const opens = closing === "" && selfClosing === "";
    textStart = opens ? tag.index + whole.length : -1;
  }
  return out.join("").trim();
}

// What an element other than a text run contributes: a tab, a line break, or
// the end of a paragraph. An opening `<w:p>` contributes nothing.
function breakFor(name: string, closing: boolean): string {
  if (name === "w:tab") {
    return "\t";
  }
  if (name === "w:br" || name === "w:cr") {
    return "\n";
  }
  return closing ? "\n" : "";
}

const XML_ENTITIES: Readonly<Record<string, string>> = {
  amp: "&",
  lt: "<",
  gt: ">",
  quot: '"',
  apos: "'",
};

function decodeXmlEntities(text: string): string {
  return text.replace(
    /&(#x[0-9a-f]+|#[0-9]+|[a-z]+);/gi,
    (whole: string, body: string) => {
      if (body.startsWith("#x") || body.startsWith("#X")) {
        return String.fromCodePoint(Number.parseInt(body.slice(2), 16));
      }
      if (body.startsWith("#")) {
        return String.fromCodePoint(Number.parseInt(body.slice(1), 10));
      }
      return XML_ENTITIES[body.toLowerCase()] ?? whole;
    },
  );
}

// A zip reader for exactly one named entry. The central directory at the end
// of the archive is the authority for sizes and offsets — the local header's
// own sizes may be deferred to a data descriptor, which Word does write. Zip64
// is not handled: a document whose body part needs it is not a writing sample.
const END_OF_CENTRAL_DIRECTORY = 0x06054b50;
const CENTRAL_DIRECTORY_ENTRY = 0x02014b50;
const END_RECORD_LENGTH = 22;
const ZIP_COMMENT_MAX = 0xffff;
const ZIP_STORED = 0;
const ZIP_DEFLATED = 8;

async function zipEntry(
  archive: ArrayBuffer,
  wanted: string,
): Promise<Uint8Array<ArrayBuffer> | null> {
  const view = new DataView(archive);
  const end = endOfCentralDirectory(view);
  if (end === -1) {
    return null;
  }
  const entries = view.getUint16(end + 10, true);
  const names = new TextDecoder();
  let offset = view.getUint32(end + 16, true);
  for (let i = 0; i < entries; i++) {
    if (view.getUint32(offset, true) !== CENTRAL_DIRECTORY_ENTRY) {
      return null;
    }
    const method = view.getUint16(offset + 10, true);
    const compressedSize = view.getUint32(offset + 20, true);
    const nameLength = view.getUint16(offset + 28, true);
    const extraLength = view.getUint16(offset + 30, true);
    const commentLength = view.getUint16(offset + 32, true);
    const localHeader = view.getUint32(offset + 42, true);
    const name = names.decode(new Uint8Array(archive, offset + 46, nameLength));
    if (name === wanted) {
      const dataStart =
        localHeader +
        30 +
        view.getUint16(localHeader + 26, true) +
        view.getUint16(localHeader + 28, true);
      const data = new Uint8Array(archive, dataStart, compressedSize);
      if (method === ZIP_STORED) {
        return data;
      }
      return method === ZIP_DEFLATED ? inflateRaw(data) : null;
    }
    offset += 46 + nameLength + extraLength + commentLength;
  }
  return null;
}

// The end record sits last, behind an archive comment of up to 64 KiB, so it
// is found by scanning back from the tail for its signature.
function endOfCentralDirectory(view: DataView): number {
  const floor = Math.max(
    0,
    view.byteLength - END_RECORD_LENGTH - ZIP_COMMENT_MAX,
  );
  for (let at = view.byteLength - END_RECORD_LENGTH; at >= floor; at--) {
    if (view.getUint32(at, true) === END_OF_CENTRAL_DIRECTORY) {
      return at;
    }
  }
  return -1;
}

async function inflateRaw(
  data: Uint8Array<ArrayBuffer>,
): Promise<Uint8Array<ArrayBuffer>> {
  const deflated = new ReadableStream<BufferSource>({
    start(controller) {
      controller.enqueue(data);
      controller.close();
    },
  });
  const inflated = deflated.pipeThrough(new DecompressionStream("deflate-raw"));
  return new Uint8Array(await new Response(inflated).arrayBuffer());
}
