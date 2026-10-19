import { parse } from "node-html-parser";

const SPDX = "SPDX-License-Identifier: BUSL-1.1";

const COPYRIGHT = "SPDX-FileCopyrightText: 2026 Gradion";

/**
 * inlineDocument folds the emitted entry chunk and stylesheet into the shell.
 *
 * The licence header goes in an HTML comment, the one place it survives.
 * esbuild strips every comment from the script, SPDX lines included, even with
 * minify off, and `legalComments` does not reach them. A third party receives
 * this artifact, so the header belongs on it.
 */
export function inlineDocument(html: string, js: string, css: string): string {
  refuseSelfClosingText(js, "script");
  const styles = stripCSSComments(css);
  refuseSelfClosingText(styles, "style");
  const root = parse(html, { comment: true });
  // Removed rather than rewritten: whatever the bundler linked is being folded
  // in, and the admission check refuses a leftover reference.
  for (const node of root.querySelectorAll("*")) {
    const tag = node.rawTagName?.toLowerCase() ?? "";
    if (tag === "link" || (tag === "script" && node.hasAttribute("src"))) {
      node.remove();
    }
  }
  // Spliced into the serialized shell rather than parsed as nodes. A script body
  // pushed through an HTML parser depends on its raw-text handling being right.
  let shell = root.toString();
  shell = spliceBefore(
    shell,
    "</head>",
    styles === "" ? "" : `<style>\n${styles}\n</style>\n`,
  );
  shell = spliceBefore(
    shell,
    "</body>",
    js === "" ? "" : `<script>\n${js}\n</script>\n`,
  );
  return stampRevision(stampLicence(shell), buildRevision());
}

/**
 * buildRevision is the commit both images are built from, passed identically to
 * the api and the web builds so the two halves can be compared. Read from the
 * environment rather than a vite `define`, because this runs in the build
 * process itself rather than in the bundled code.
 */
export function buildRevision(): string {
  return process.env.MARGINCE_BUILD_REVISION ?? "";
}

/**
 * stampRevision writes the build this document came from into the document, as
 * an HTML comment.
 *
 * A comment, because nowhere else works: esbuild strips every comment from the
 * script, and the parsed validator would have to learn to allow a `<meta>`.
 * The api reads it to report skew. It is never an integrity signature, and
 * never a reason to refuse.
 *
 * An absent revision writes nothing. A local build has no meaningful SHA (the
 * worktree is dirty). A stamp of "" or "dev" would need a special case on the
 * api's read side as well as the write side.
 */
function stampRevision(shell: string, revision: string): string {
  if (revision === "" || revision === "dev") return shell;
  return shell.replace(
    LICENCE_END,
    `${LICENCE_END}\n<!-- ${REVISION_MARKER}${revision} -->`,
  );
}

/** The marker the api reads the revision back out of. One spelling, here. */
const REVISION_MARKER = "margince-build-revision: ";

const LICENCE_END = "-->";

/**
 * stampLicence puts the SPDX lines in an HTML comment after the doctype.
 *
 * A comment ahead of the doctype puts the browser into quirks mode, which would
 * change how every rule in the stylesheet is applied. The header is the first
 * thing a human reads, not the first thing the parser does.
 */
function stampLicence(shell: string): string {
  const doctype = /^\s*<!doctype[^>]*>/i.exec(shell);
  if (doctype === null) {
    throw new Error(
      "mcp-apps: the built document has no doctype to stamp the licence after",
    );
  }
  const header = `\n<!--\n${SPDX}\n${COPYRIGHT}\n-->`;
  return (
    shell.slice(0, doctype[0].length) + header + shell.slice(doctype[0].length)
  );
}

/**
 * spliceBefore inserts ahead of a closing tag the shell is required to have. It
 * throws rather than appending on a miss. A script that landed somewhere else
 * is worse than a failed build.
 */
function spliceBefore(shell: string, marker: string, insert: string): string {
  if (insert === "") return shell;
  const at = shell.lastIndexOf(marker);
  if (at < 0) {
    throw new Error(`mcp-apps: the shell has no ${marker} to inline into`);
  }
  return shell.slice(0, at) + insert + shell.slice(at);
}

/**
 * stripCSSComments removes CSS comments without reaching inside string literals.
 *
 * A regex would rewrite the legal `content: "/* label *​/"` to `content: ""`.
 * tokens.css and brand.css carry prose, URLs included, that the raw-read
 * admission check would read as code. So this scans, tracking quote state.
 */
function stripCSSComments(css: string): string {
  let out = "";
  let i = 0;
  while (i < css.length) {
    const c = css[i];
    if (c === '"' || c === "'") {
      const end = endOfString(css, i);
      out += css.slice(i, end);
      i = end;
      continue;
    }
    if (c === "/" && css[i + 1] === "*") {
      i = endOfComment(css, i);
      continue;
    }
    out += c;
    i++;
  }
  return out.trim();
}

/** endOfString answers the index just past the closing quote, honouring
 *  escapes: an escaped quote does not end the literal. */
function endOfString(css: string, start: number): number {
  const quote = css[start];
  for (let i = start + 1; i < css.length; i++) {
    if (css[i] === "\\") {
      i++;
      continue;
    }
    if (css[i] === quote) return i + 1;
  }
  return css.length;
}

/** endOfComment answers the index just past the closing delimiter. An
 *  unterminated comment swallows the rest, which is what a browser does too. */
function endOfComment(css: string, start: number): number {
  const end = css.indexOf("*/", start + 2);
  return end < 0 ? css.length : end + 2;
}

/**
 * refuseSelfClosingText fails the build rather than emitting a document whose
 * inline block ends early. The sequence can occur only inside a literal in
 * valid JavaScript or CSS. Escaping it would make the bytes served differ from
 * the bytes built.
 */
function refuseSelfClosingText(text: string, tag: string): void {
  if (text.toLowerCase().includes(`</${tag}`)) {
    throw new Error(
      `mcp-apps: the inline ${tag} contains "</${tag}", which would end the block early and ` +
        "leave the rest of it as page text — rewrite the literal that produces it",
    );
  }
}
