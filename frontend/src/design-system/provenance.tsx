// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { providerName } from "../format/connectorprovider";
import { sourceSystemName } from "../format/sourcesystem";
import { isMessageKey, type Translator, useT } from "../i18n";
import { Badge } from "./atoms";
import "./trust.css";

// Where a value came from, as a reader can act on it: the vocabulary, the tag
// that draws it, and the words each arm reads as.
//
// It sits beside trust.tsx rather than inside it because the two answer
// different questions. The trust primitives are about a value's STATE — staged
// or real, how sure, on what evidence, and the Accept/Edit/Dismiss triad that
// resolves it. This is about a value's ORIGIN, which every one of those states
// carries and which four screens render without staging anything at all.

// Provenance is an agent (`agent:capture`), a connector (`connector:gmail`), a
// job the installation ran itself (`system:contact_auto_enrich`), a human or a
// buyer. Those are the shapes captured_by can take. An import is one more arm,
// and the last arm is a row that records none of them. A reader has to tell
// which kind of thing produced a value, so each is its own arm. A scheduled
// sweep announced as an AI agent misdescribes both.
//
// `buyer` is the contact on the other side of a Deal Room: outside the
// company, holding no seat and named in no member directory. It is its own
// arm rather than a `human` one because a reader cannot ask a buyer the way
// they can ask a colleague, and it is not `unknown` because that arm means
// nobody recorded a source — here the source IS recorded, and it is a contact.
// Nothing to name today: a Deal Room participant resolves to no display name on
// the read path, so the tag says the kind, the way `agent` and `system` do.
//
// `import` is read from the record's `source` stamp, because captured_by on an
// imported row names only the administrator who ran the import. It reads
// "Imported from <upload name>", or "Imported" when the upload has no name.
//
// `human` carries whether that human is the reader. "Typed by you" over a
// colleague's entry is a false statement about who to ask, and it was also
// what an unattributed row said: the two cases a reader most needs kept apart
// both read as their own handiwork.
//
// `agent` and `system` name the actor only when the wire named it. A passport
// uuid has no lookup here, so an unnamed tag says the kind and stops. The agent
// arm carries `agent`, words a caller chose, or `job`, the principal's name
// read through the catalogue.
//
// `author` is who wrote it in the system a row was MIGRATED OUT OF, and it
// outranks every other reading of the human arm. An import runs as one
// administrator, so `captured_by` names them on every row it wrote and `self`
// is true for that administrator on all of it — which is how a migration comes
// to tell one colleague they typed a decade of everybody else's
// correspondence. The author is the only field on the row that knows better,
// so where it is present it is what the tag says.
export type Provenance =
  | { kind: "agent"; agent?: string; job?: string }
  | { kind: "connector"; connector: string }
  | { kind: "system"; job?: string }
  | { kind: "human"; self: boolean; userId?: string; author?: SourceAuthor }
  | { kind: "buyer" }
  // A row the CSV importer wrote. `uploadName` is what the administrator called
  // the upload, never the record's `source` stamp that carries it.
  | { kind: "import"; uploadName?: string }
  | { kind: "unknown" };

// Who wrote an imported row where it came from, as the contract reports it.
// Taken from the generated schema rather than spelled again here: a
// hand-written copy of a generated shape drifts from the contract silently,
// and this one is read by a tag whose whole job is to be true.
export type SourceAuthor = components["schemas"]["SourceAuthor"];

export function ProvenanceTag({
  provenance,
  // How a named human renders. The design system has no record lookups, so a
  // caller that can resolve a user id to a name supplies the element; without
  // one the tag says a contact entered it without claiming which one.
  renderUser,
}: Readonly<{
  provenance: Provenance;
  renderUser?: (userId: string) => ReactNode;
}>) {
  const t = useT();
  // Indigo is the claim that a model wrote it, so only the agent arm wears it.
  // A connector copies what a mailbox already held and a system job runs a
  // rule nobody inferred: drawn in the AI tone, either would tell a reader a
  // model decided something. Every other arm is the neutral badge, told apart
  // by its words.
  return (
    <Badge tone={provenance.kind === "agent" ? "ai" : "default"}>
      {provenanceLabel(provenance, t, renderUser)}
    </Badge>
  );
}

/**
 * The provenance as words alone, for a meta line that names where a value came
 * from beside other plain words (a fact row's source). The tag above is the
 * same words on a badge, for a value that stands on its own.
 */
export function provenanceLabel(
  provenance: Provenance,
  t: Translator,
  renderUser: ((userId: string) => ReactNode) | undefined,
): ReactNode {
  switch (provenance.kind) {
    case "agent": {
      const agent =
        provenance.agent ??
        (provenance.job &&
          (jobLabel("agentJob", provenance.job, t) ??
            extensionJobLabel(provenance.job)));
      return agent ? t("trust.agentTag", { agent }) : t("trust.agentUnnamed");
    }
    case "system": {
      const job = provenance.job && jobLabel("systemJob", provenance.job, t);
      return job ? t("trust.systemTag", { job }) : t("trust.systemUnnamed");
    }
    case "connector":
      return t("trust.connectorTag", {
        connector: providerName(provenance.connector, t),
      });
    case "buyer":
      return t("trust.typedByBuyer");
    case "import":
      return importLabel(provenance, t);
    case "unknown":
      return t("trust.sourceUnknown");
    case "human":
      return humanLabel(provenance, t, renderUser);
  }
}

/**
 * The import arm in words. Exported for a reader that shows a record's source
 * as plain text (a lead's Source fact) rather than as a tag.
 */
export function importLabel(
  provenance: Extract<Provenance, { kind: "import" }>,
  t: Translator,
): string {
  // A connector import is named by the system's key ("hubspot"); the reader
  // sees the product's own spelling, the same one an author's "via" gets.
  return provenance.uploadName
    ? t("trust.importedFrom", {
        uploadName: sourceSystemName(provenance.uploadName),
      })
    : t("trust.imported");
}

/**
 * jobLabel names a system pass or an agent by its words in the catalogue, never
 * by its key. Every system principal the backend binds has a `systemJob.<name>`
 * message and every core agent principal an `agentJob.<name>` one, held by
 * backend/gates/frontendjoblabels_test.go and frontendagentlabels_test.go. Hyphens read as underscores
 * (`system:lead-sla-scan`). A name with no message gets no label, and the tag
 * names only the kind.
 */
function jobLabel(
  family: "systemJob" | "agentJob",
  job: string,
  t: Translator,
): string | undefined {
  const pass = family === "systemJob" ? job.split(":")[0] : job;
  const name = pass?.replaceAll("-", "_");
  // Each family spelled whole, so the orphan-key sweep sees both prefixes.
  const key = family === "systemJob" ? `systemJob.${name}` : `agentJob.${name}`;
  return isMessageKey(key) ? t(key) : undefined;
}

// How a row somebody typed reads, which is four different answers rather than
// one. They are asked in this order because each is true of a narrower set of
// rows than the last, and the wrong order is what put "Typed by you" on a
// colleague's work.
function humanLabel(
  provenance: Extract<Provenance, { kind: "human" }>,
  t: Translator,
  renderUser: ((userId: string) => ReactNode) | undefined,
): ReactNode {
  // FIRST, because it is the only field that knows. On an imported row `self`
  // answers "is the reader the administrator who ran the import" — true of
  // every one of those rows for that one reader, and not a question about who
  // wrote the thing.
  if (provenance.author) {
    const { display_name: name, via } = provenance.author;
    return via
      ? t("trust.loggedInByVia", { via: sourceSystemName(via), name })
      : t("trust.loggedInBy", { name });
  }
  if (provenance.self) {
    return t("trust.typedByYou");
  }
  const named = provenance.userId ? renderUser?.(provenance.userId) : undefined;
  return named ? (
    <>
      {t("trust.typedByPrefix")} {named}
    </>
  ) : (
    t("trust.typedByHuman")
  );
}

/**
 * The core catalogue cannot know the jobs an extension unit declares. So an
 * extension dispatcher (`agent:ext_openchannel_drain`) is named by its unit
 * and job in words. The prefix mirrors the backend's extension.NamespacePrefix,
 * and frontendagentlabels_test.go holds the two together.
 */
const EXTENSION_JOB_PREFIX = "ext_";

function extensionJobLabel(job: string): string | undefined {
  if (!job.startsWith(EXTENSION_JOB_PREFIX)) {
    return undefined;
  }
  const words = job.slice(EXTENSION_JOB_PREFIX.length).replaceAll("_", " ");
  return words.trim() || undefined;
}
