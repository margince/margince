// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { type Translator, useT } from "../i18n";
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
// job the installation ran itself (`system:contact_auto_enrich`), a human, or a
// buyer — the shapes captured_by can take, plus the honest last arm for a row
// that records none of them. A reader has to be able to tell WHICH KIND of
// thing produced a value, so each is its own arm: a scheduled sweep announced
// as an AI agent misdescribes both.
//
// `buyer` is the contact on the other side of a Deal Room: outside the
// company, holding no seat and named in no member directory. It is its own
// arm rather than a `human` one because a reader cannot ask a buyer the way
// they can ask a colleague, and it is not `unknown` because that arm means
// nobody recorded a source — here the source IS recorded, and it is a contact.
// Nothing to name today: a Deal Room participant resolves to no display name on
// the read path, so the tag says the kind, the way `agent` and `system` do.
//
// `human` carries whether that human is the reader. "Typed by you" over a
// colleague's entry is a false statement about who to ask, and it was also
// what an unattributed row said: the two cases a reader most needs kept apart
// both read as their own handiwork.
//
// `agent` and `system` name the actor only when the wire named it. Neither is
// required, because the id behind an agent may be a passport uuid and there are
// no record lookups here to resolve it: an unnamed tag says the kind and stops,
// which is more than an identifier tells a reader and all of it is true.
//
// `author` is who wrote it in the system a row was MIGRATED OUT OF, and it
// outranks every other reading of the human arm. An import runs as one
// administrator, so `captured_by` names them on every row it wrote and `self`
// is true for that administrator on all of it — which is how a migration comes
// to tell one colleague they typed a decade of everybody else's
// correspondence. The author is the only field on the row that knows better,
// so where it is present it is what the tag says.
export type Provenance =
  | { kind: "agent"; agent?: string }
  | { kind: "connector"; connector: string }
  | { kind: "system"; job?: string }
  | { kind: "human"; self: boolean; userId?: string; author?: SourceAuthor }
  | { kind: "buyer" }
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
    case "agent":
      return provenance.agent
        ? t("trust.agentTag", { agent: provenance.agent })
        : t("trust.agentUnnamed");
    case "system":
      return provenance.job
        ? t("trust.systemTag", { job: provenance.job })
        : t("trust.systemUnnamed");
    case "connector":
      return t("trust.connectorTag", { connector: provenance.connector });
    case "buyer":
      return t("trust.typedByBuyer");
    case "unknown":
      return t("trust.sourceUnknown");
    case "human":
      return humanLabel(provenance, t, renderUser);
  }
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
      ? t("trust.loggedInByVia", { via, name })
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
