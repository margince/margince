import type { components } from "../api/schema";
import type { Provenance, RelaySend, SourceAuthor } from "./trust";

type PrincipalAttribution = components["schemas"]["PrincipalAttribution"];
type ControllerSend = components["schemas"]["ControllerSend"];

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
  // The row's own `source` stamp. An import writes it inside the importer's
  // namespace, and captured_by there names only the administrator who ran it.
  stamp?: string | null,
  // The record's `imported_from`: the server's own answer to "did an import
  // write this". It holds for rows whose `source` carries no stamp, such as
  // those written through the import API.
  importedFrom?: string | null,
): Provenance {
  // An author already says where the row came from, and better. Without one,
  // the administrator's seat would read "typed by you" over a migrated row.
  const imported = author ? undefined : importArm(stamp, importedFrom);
  if (imported) {
    return imported;
  }
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

/**
 * An agent's tag with what its audit row adds: the tool it was typed through
 * and the colleague it worked for. captured_by and actor_id name only a
 * passport, so without the row the tag can say neither.
 */
export function withCreator(
  provenance: Provenance,
  attribution: PrincipalAttribution | undefined,
): Provenance {
  if (provenance.kind !== "agent" || attribution?.actor_type !== "agent") {
    return provenance;
  }
  return {
    ...provenance,
    agent: attribution.agent_client ?? provenance.agent,
    forName: attribution.on_behalf_of_name ?? undefined,
  };
}

// A mail the installation's relay sent reads as the relay's, never as the
// member's own; that member is "you" only when captured_by names the reader.
export function withControllerSend(
  provenance: Provenance,
  send: ControllerSend | null | undefined,
  viewerUserId?: string,
): Provenance {
  if (!send) {
    return provenance;
  }
  const member = provenance.kind === "human" ? provenance.userId : undefined;
  return relayProvenance(send, relayMemberFor(member, viewerUserId));
}

/** The relay arm for a caller that knows the member another way. */
export function relayProvenance(
  send: ControllerSend,
  member: RelaySend["member"],
): Provenance {
  return { kind: "system", relay: { member, fromAddress: send.from_address } };
}

/** Whom a relay send was for: the reader only when the member is the reader. */
export function relayMemberFor(
  memberId: string | null | undefined,
  viewerId: string | undefined,
): RelaySend["member"] {
  if (!memberId || !viewerId) {
    return "unknown";
  }
  return memberId === viewerId ? "self" : "colleague";
}

// What an `agent:<rest>` principal is called on screen. A passport uuid names
// nothing on this side, so the tag says only that an agent acted. A named tool
// (`agent:signal-scan`) is handed on as `job`, which the tag reads through the
// catalogue; the identifier itself is never printed.
function agentProvenance(rest: string): Provenance {
  const named = rest === "" || OPAQUE_ID.test(rest) ? undefined : rest;
  return { kind: "agent", job: named };
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

/**
 * The importer's reserved namespace. It mirrors the backend's
 * `provenance.ReservedSourceSystemPrefix`; no client write path can spell it,
 * so a `source` that starts with it was written by an import.
 */
const IMPORT_NAMESPACE = "mirror:";

/** The connector segment the CSV importer writes after the namespace. */
const IMPORT_CONNECTOR = "csv";

/** The objects an import stamps after its upload name. */
const IMPORT_OBJECTS = new Set([
  "company",
  "contact",
  "lead",
  "deal",
  "activity",
]);

/**
 * The import arm a row's `source` stamp reads as, or undefined when no import
 * wrote it. The CSV importer stamps `mirror:csv[:<upload>]:<object>:<id>`.
 */
export function importProvenance(
  stamp: string | null | undefined,
): Extract<Provenance, { kind: "import" }> | undefined {
  if (!stamp?.startsWith(IMPORT_NAMESPACE)) {
    return undefined;
  }
  const [connector, ...tail] = stamp.slice(IMPORT_NAMESPACE.length).split(":");
  return { kind: "import", uploadName: uploadNameOf(connector, tail) };
}

// The stamp is read first, and the wire's answer fills in where it has none.
function importArm(
  stamp: string | null | undefined,
  importedFrom: string | null | undefined,
): Extract<Provenance, { kind: "import" }> | undefined {
  return importProvenance(stamp) ?? importedFromProvenance(importedFrom);
}

/**
 * The import arm the wire's `imported_from` reads as. Null or absent means no
 * import wrote the row; an empty string is an import that was given no name.
 */
function importedFromProvenance(
  importedFrom: string | null | undefined,
): Extract<Provenance, { kind: "import" }> | undefined {
  if (importedFrom === null || importedFrom === undefined) {
    return undefined;
  }
  return { kind: "import", uploadName: importedFrom || undefined };
}

// The upload is named only when an object and an id follow it.
// So an unnamed upload whose id spells an object ("mirror:csv:company:contact")
// has no name.
// An upload called "contact" keeps its name.
// An object the browser does not know yet reads "Imported", not a name.
function uploadNameOf(
  connector: string | undefined,
  tail: readonly string[],
): string | undefined {
  if (
    connector !== IMPORT_CONNECTOR ||
    tail.length < 3 ||
    !IMPORT_OBJECTS.has(tail[1] ?? "")
  ) {
    return undefined;
  }
  return tail[0] || undefined;
}
