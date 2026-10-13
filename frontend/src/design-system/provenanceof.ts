import type { Provenance, SourceAuthor } from "./trust";

// captured_by is server-stamped from the authenticated principal, so it carries
// that principal's kind. The contract lists four
// ("human:<uuid> | agent:<id> | connector:<name> | system:<id>"). A Deal Room
// participant's own writes add a fifth, `buyer:<participant uuid>`.
// Each reads as the kind of actor it is. A connector reads as a connector, a
// background job as the system, an agent as an agent, a buyer as a buyer.
// Telling those apart is why a provenance tag is on the screen.
// A kind this app does not know is `unknown` rather than any of them.
// Naming the wrong kind of actor is a claim about who to ask. "Not recorded"
// is the only thing still true when the string is unreadable.
//
// A human is only "you" when the id is the reader's. Without a viewer id the
// human branch stays unnamed rather than guessing. A caller that cannot say
// who is reading cannot claim the reader typed it. An absent captured_by is
// `unknown` and says so. Showing it as the reader's own typing would be an
// attribution nobody can check.
//
// The kind is the first segment, and everything after it belongs to that kind.
// So the leading colon is found by index. `split(":", 2)` truncates at the
// limit instead of keeping the remainder in the last element. Then
// `connector:ext:dispact-connector:<uuid>` parses to `["connector", "ext"]`, and
// every extension unit's tag reads the literal word "ext".
//
// Nothing opaque is handed on for a tag to print. A uuid names nothing a reader
// can act on, so it is dropped rather than rendered. A passport id leaves the
// agent tag unnamed, and a connector's member id is dropped a segment at a time.
// The human remainder is the exception and is kept whole. It is compared
// against the reader's own id and never printed (the tag resolves it through
// the caller's `renderUser` or says a contact entered it). An id truncated to
// its first segment would make a colleague's entry read "typed by you".
export function provenanceOf(
  capturedBy: string | undefined,
  viewerUserId?: string,
  // Who wrote it in the system it was imported from, when the row carries one.
  // An import runs as a single administrator, so captured_by names that one
  // seat on every row it wrote. That is true, and useless as a statement about
  // authorship. The author is the field that knows, and the tag prefers it.
  author?: SourceAuthor | null,
): Provenance {
  if (!capturedBy) {
    return { kind: "unknown" };
  }
  const separator = capturedBy.indexOf(":");
  const source = separator > 0 ? capturedBy.slice(0, separator) : capturedBy;
  const rest = separator > 0 ? capturedBy.slice(separator + 1) : "";
  if (source === "human") {
    const userId = rest || undefined;
    return {
      kind: "human",
      self: Boolean(viewerUserId) && userId === viewerUserId,
      userId,
      author: author ?? undefined,
    };
  }
  if (source === "buyer") {
    // The other side of a Deal Room: a contact, outside the company and in
    // no member directory. The human arm would send a reader looking them up,
    // and `unknown` says nobody recorded a source; neither is true of them.
    // What follows the kind is the participant uuid. It is opaque, and is
    // dropped here for the same reason a passport id is.
    return { kind: "buyer" };
  }
  if (source === "connector") {
    return { kind: "connector", connector: connectorLabel(rest) || source };
  }
  if (source === "agent") {
    return agentProvenance(rest);
  }
  if (source === "system") {
    // The installation's own processing: a scheduled sweep, a backfill, an
    // anonymous public endpoint. Its id is a job name, and a bare `system` with
    // no id at all is a job that never said which one it was.
    return { kind: "system", job: rest || undefined };
  }
  return { kind: "unknown" };
}

// What an `agent:<rest>` principal is called on screen.
//
// A passport call stamps `agent:<passport_id>`, a uuid with no name behind it
// on this side. The design system holds no record lookups, so there is
// nothing here to resolve it to. The tag then says only what the wire
// says (an agent produced this) instead of printing an identifier. A named
// tool (`agent:enrich`) keeps its name.
function agentProvenance(rest: string): Provenance {
  const named = rest === "" || OPAQUE_ID.test(rest) ? undefined : rest;
  return { kind: "agent", agent: named && humanizeAgent(named) };
}

// A tool's own name, in words.
//
// The wire spells a job the way the code does: `capture_counterparty_verdict`.
// Printing that at a reader shows them the plumbing and calls it information.
// The underscores go and the name stays, because the name identifies the tool.
//
// The case is left alone. A one-word tool reads as `capture`, and changing
// that is a separate decision from removing an identifier's punctuation.
function humanizeAgent(name: string): string {
  const words = name.replaceAll("_", " ").trim();
  return words === "" ? name : words;
}

/** A uuid: an identifier with no name in it, so no tag may print it. */
const OPAQUE_ID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** The marker that says the rest of a connector principal names an extension. */
const EXTENSION_MARKER = "ext";

/**
 * Which connector a `connector:` principal names, from what follows the kind.
 *
 * Two grammars end up in the same column. A built-in connector writes
 * `<system>:<user-uuid>`, an extension unit `ext:<unit>[:<user-uuid>]`. Either way the
 * connector is one segment and the rest is the member whose grant it ran under.
 * So the label is taken a segment at a time. A uuid tail in a provenance tag
 * names nothing a reader can act on, and the unit is what tells
 * `dispact-connector` from `zalo-oa`.
 *
 * The marker on its own names no unit, so it labels nothing. The empty answer
 * sends the caller back to the kind, and the tag reads "connector" rather than
 * "ext". That word is a segment of the grammar and names nothing that ran.
 *
 * The unit is the raw directory name. The transport directory resolves the
 * provider-id grammar (`ext:<unit>:<system>`), a different namespace from
 * this one. So there is no nicer label to look up here yet.
 */
function connectorLabel(rest: string): string {
  const [head = "", ...tail] = rest.split(":");
  return head === EXTENSION_MARKER ? (tail[0] ?? "") : head;
}
