// Display-side reading of a captured mail body. The stored body is the whole
// message — signature, quoted history and all — because the enrichment pass
// mines the sign-off for contact details, so nothing may be trimmed at rest.
// What a reader wants on a timeline row is the sentence the sender wrote, and
// that is what this file finds. The rest is handed back separately and folded
// behind a control rather than dropped: a heuristic that guesses wrong must
// stay one click from being wrong in public.
//
// A deliberate mirror of backend/internal/modules/activities/emailtext.go: the
// server composes a row's preview and this folds the tail in the drawer, so a
// table that drifted would make a row disagree with the message it opens.
// backend/gates/emailsplitterparity_test.go compares the two in both
// directions — an entry added on one side and not the other is a red build.

export type EmailBodyParts = {
  /** The `From:/To:` preamble capture folds into the body, when present. */
  header: string;
  /** What the sender actually wrote. Never empty when the body was not. */
  main: string;
  /**
   * Signature and quoted history, collapsed behind a control.
   *
   * Kept as one string because callers that only need "is there a tail" ask
   * this. What the tail IS — a sign-off, a quoted reply, or a sign-off with a
   * reply under it — is `tail`, and a caller labelling the control must read
   * that instead: folding a sender's own name behind "show quoted history"
   * hides it under a promise of history that is not there.
   */
  trimmed: string;
  /**
   * What the trimmed part starts with. "none" when there is no tail at all.
   *
   * A signature and a quote are trimmed for different reasons and read
   * differently to a contact: the sign-off is the sender still speaking, and
   * the quote is an older message. One label for both was the defect.
   */
  tail: "none" | "signature" | "quote";
};

// The preamble capture's mail mapper prepends verbatim. Peeled before any
// heuristic runs, or the `From:` line reads as an Outlook reply header and
// folds the entire message.
const PREAMBLE = /^From:[^\n]*\nTo:[^\n]*\n\n/;

// RFC 3676 §4.3: the sign-off delimiter is a line of exactly two hyphens.
// Trailing whitespace is allowed because mail clients strip it inconsistently.
const SIGNATURE_DELIMITER = /^--\s*$/;

// Attribution lines that introduce a quoted reply. Bounded repetition keeps a
// pathological body from backtracking.
const ATTRIBUTION = [
  /^On\s.{1,200}\swrote:\s*$/,
  /^Am\s.{1,200}\sschrieb\s?.{0,120}:\s*$/,
  /^Vào\s.{1,200}\sđã viết:\s*$/,
  /^Ngày\s.{1,200}\sđã viết:\s*$/,
  /^-{2,}\s*(Original Message|Ursprüngliche Nachricht|Thư gốc|Tin nhắn gốc)/i,
  /^_{5,}\s*$/,
];

// Lines that open mail the sender passes on. It is new to the reader, so it is
// never folded away as history.
const FORWARD = [
  /^-{2,}\s*(Forwarded message|Weitergeleitete Nachricht|Thư được chuyển tiếp|Thư đã chuyển tiếp|Tin nhắn được chuyển tiếp)/i,
  /^Begin forwarded message:\s*$/i,
  /^(Thư được chuyển tiếp|Thư đã chuyển tiếp|Tin nhắn được chuyển tiếp):\s*$/i,
  /^Anfang der weitergeleiteten (E-Mail|Nachricht)/i,
];

// One field of the header block under a forward line, and how many lines the
// block may run.
const FORWARD_HEADER_LINE = /^\*{0,2}[^\s:>*][^:]{0,30}:/;
const QUOTED_HEADER_REACH = 8;

// The Outlook top-reply block: a From:/Von: line whose neighbours carry the
// sent-date field. Recognised as a pair because "Von:" alone is ordinary prose
// in German mail.
const REPLY_HEADER_FROM = /^\*{0,2}(Von|From|Từ):\*{0,2}\s/;
const REPLY_HEADER_SENT =
  /^\*{0,2}(Gesendet|Sent|Datum|Date|Đã gửi|Ngày):\*{0,2}\s/;

// Sign-offs, matched only in the trailing region. The multi-word forms may open
// a line, since a name usually follows on the same one.
const SIGN_OFF_PREFIXES = [
  "mit freundlichen grüßen",
  "mit freundlichem gruß",
  "mit besten grüßen",
  "freundliche grüße",
  "viele grüße",
  "beste grüße",
  "herzliche grüße",
  "liebe grüße",
  "schöne grüße",
  "best regards",
  "kind regards",
  "warm regards",
  "many thanks",
  "thanks and regards",
  "diese e-mail wurde von avast",
];

// Mobile-client boilerplate. Matched against the WHOLE line rather than as a
// prefix: "Sent from my perspective, the contract is not ready" opens a
// sentence with the same three words and is the message, not the footer.
const SIGN_OFF_LINES = [
  /^von meinem (iphone|ipad|android|mobilteil|samsung)\b.*gesendet$/,
  /^sent from my \w[\w\s]{0,30}$/,
  /^gesendet mit \w[\w\s]{0,30}$/,
  /^get outlook for \w+$/,
];

// The short forms. A whole-line match only: "LG" opens a sentence about a
// washing machine as readily as it closes a mail, and "Best" mid-paragraph is
// not a sign-off at all.
const SIGN_OFF_EXACT = new Set([
  "mfg",
  "vg",
  "lg",
  "vlg",
  "best",
  "regards",
  "cheers",
  "thanks",
  "thank you",
  "danke",
  "gruß",
  "grüße",
  "bg",
]);

// How far up from the end a sign-off is looked for. A sign-off belongs to the
// closing of a mail; the same words in the opening paragraph are prose, and
// scanning the whole body folds the message away on the strength of one line.
const TRAILING_SCAN_LINES = 15;

function isSignOff(line: string): boolean {
  const normalized = line
    .trim()
    .toLowerCase()
    // The lookbehind is what keeps this linear. Without it the engine retries
    // the trailing run from every position in the line, which is quadratic on a
    // line that is all punctuation — and an inbound mail body is not ours.
    .replace(/(?<![,.!])[,.!]+$/, "");
  if (!normalized) {
    return false;
  }
  if (SIGN_OFF_EXACT.has(normalized)) {
    return true;
  }
  if (SIGN_OFF_LINES.some((pattern) => pattern.test(normalized))) {
    return true;
  }
  return SIGN_OFF_PREFIXES.some((prefix) => normalized.startsWith(prefix));
}

function startsQuote(lines: readonly string[], index: number): boolean {
  const line = lines[index] ?? "";
  if (line.startsWith(">")) {
    return true;
  }
  if (ATTRIBUTION.some((pattern) => pattern.test(line))) {
    return true;
  }
  // The Outlook block: the sent-date field within the next few lines is what
  // separates a reply header from a sentence that opens "Von:".
  if (index > 0 && REPLY_HEADER_FROM.test(line)) {
    return lines
      .slice(index + 1, index + 5)
      .some((next) => REPLY_HEADER_SENT.test(next));
  }
  return false;
}

const isBlank = (line: string | undefined): boolean => !line?.trim();

// The index after the ">" lines a quote at `index` opens, or -1 when the quote
// carries no ">" lines.
function markedRunEnd(lines: readonly string[], index: number): number {
  let first = index;
  if (!lines[first]?.startsWith(">")) {
    first += 1;
    while (first < lines.length && isBlank(lines[first])) {
      first += 1;
    }
    if (!lines[first]?.startsWith(">")) {
      return -1;
    }
  }
  let after = first;
  while (
    after < lines.length &&
    (lines[after]?.startsWith(">") || isBlank(lines[after]))
  ) {
    after += 1;
  }
  return after;
}

// The index where a forwarded mail's own lines are read from. It passes the
// header block, whose From and Date fields would read as an Outlook reply
// header, and a mail forwarded with ">" on every line.
function forwardedTextStart(lines: readonly string[], from: number): number {
  let start = from;
  while (start < lines.length && isBlank(lines[start])) {
    start += 1;
  }
  let end = start;
  while (
    end < lines.length &&
    end - start < QUOTED_HEADER_REACH &&
    FORWARD_HEADER_LINE.test(lines[end] ?? "")
  ) {
    end += 1;
  }
  while (
    end < lines.length &&
    (lines[end]?.startsWith(">") || isBlank(lines[end]))
  ) {
    end += 1;
  }
  return end;
}

function isBlankFrom(lines: readonly string[], from: number): boolean {
  for (let i = from; i < lines.length; i += 1) {
    if (!isBlank(lines[i])) {
      return false;
    }
  }
  return true;
}

// The attribution line directly above a quoted block introduces it, so it
// travels with the quote rather than trailing the message.
function attributedStart(lines: readonly string[], index: number): number {
  const above = lines[index - 1]?.trim() ?? "";
  const marked = lines[index]?.startsWith(">") ?? false;
  return marked && index > 0 && above.endsWith(":") ? index - 1 : index;
}

/**
 * ownTextEnd is the index of the first line of quoted earlier mail, or
 * `lines.length` when the sender wrote or forwarded every line.
 *
 * A line a mail program writes above quoted mail ends the text. A quote marked
 * with ">" ends it only when nothing follows, so an inline reply stays whole.
 * The server stores the same boundary for search.
 */
function ownTextEnd(lines: readonly string[]): number {
  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i] ?? "";
    if (FORWARD.some((pattern) => pattern.test(line))) {
      i = forwardedTextStart(lines, i + 1) - 1;
      continue;
    }
    if (!startsQuote(lines, i)) {
      continue;
    }
    const after = markedRunEnd(lines, i);
    if (after < 0) {
      return i;
    }
    if (isBlankFrom(lines, after)) {
      return attributedStart(lines, i);
    }
    i = after - 1;
  }
  return lines.length;
}

/**
 * boundaryIndex is the first line that belongs to the tail rather than the
 * message, and which kind of tail it opens. A sign-off counts only above the
 * quoted earlier mail.
 *
 * `at` is -1 when the whole body is the message.
 */
function boundaryIndex(lines: readonly string[]): {
  at: number;
  kind: "signature" | "quote";
} {
  const quote = ownTextEnd(lines);
  const signOffFloor = Math.max(0, lines.length - TRAILING_SCAN_LINES);
  for (let i = 0; i < quote; i += 1) {
    const line = lines[i] ?? "";
    if (
      SIGNATURE_DELIMITER.test(line) ||
      (i >= signOffFloor && isSignOff(line))
    ) {
      return { at: i, kind: "signature" };
    }
  }
  return { at: quote < lines.length ? quote : -1, kind: "quote" };
}

/**
 * splitEmailBody separates a stored mail body into the part worth reading on a
 * row and the tail worth keeping but not showing.
 *
 * The message is never allowed to become empty: a body that is nothing but a
 * greeting is a real message, and a heuristic that folds it away has hidden the
 * mail rather than tidied it.
 */
export function splitEmailBody(body: string): EmailBodyParts {
  if (!body.trim()) {
    return { header: "", main: "", trimmed: "", tail: "none" };
  }
  const preamble = PREAMBLE.exec(body);
  const header = preamble ? preamble[0].trim() : "";
  const rest = preamble ? body.slice(preamble[0].length) : body;

  // A body that is only a preamble has no message under it. The header is the
  // whole of what was captured, so it IS the message: the invariant is that a
  // non-empty body never renders as nothing.
  if (!rest.trim()) {
    return { header: "", main: body.trim(), trimmed: "", tail: "none" };
  }

  const lines = rest.split("\n");
  const cut = boundaryIndex(lines);
  if (cut.at < 0) {
    return { header, main: rest.trim(), trimmed: "", tail: "none" };
  }
  const main = lines.slice(0, cut.at).join("\n").trim();
  if (!main) {
    return { header, main: rest.trim(), trimmed: "", tail: "none" };
  }
  return {
    header,
    main,
    trimmed: lines.slice(cut.at).join("\n").trim(),
    tail: cut.kind,
  };
}

/**
 * emailSummaryText is the message on one line, for a row title or a memory
 * entry: the sender's own words, never their sign-off and never the `From:`
 * preamble.
 */
export function emailSummaryText(body: string): string {
  const { main } = splitEmailBody(body);
  const collapsed = main.replace(/\s+/g, " ").trim();
  // A body that is only a quoted forward has no message of its own. The whole
  // text is a better title than an empty one.
  return collapsed || body.replace(/\s+/g, " ").trim();
}
