// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The records a filter sentence names instead of counting: which ids its
// clauses hold, and the names the catalogues the pickers read give them.

import { useQueries } from "@tanstack/react-query";
import { entityNameObserverQuery } from "../../components/entityref.queries";
import {
  type ReferenceOption,
  useReferenceOptions,
} from "../../components/filterreference";
import type { VocabularyField } from "../../lib/filterdata";
import { useMemberNames } from "../../lib/membernames";
import { isGroup, type Node } from "../../lib/segmentpredicate";

type ClauseRef = Readonly<{ field: string; operand: unknown }>;

/** The references a sentence names by looking the record up, not by counting. */
export const NAMED_REFERENCES = [
  "company",
  "app_user",
  "team",
  "stage",
  "pipeline",
  "project",
  "product",
] as const;
type NamedReference = (typeof NAMED_REFERENCES)[number];

export function isNamedReference(
  value: string | undefined,
): value is NamedReference {
  return NAMED_REFERENCES.some((kind) => kind === value);
}

/** The ids of each named reference that a sentence will have to name. */
export type ReferencedIds = Readonly<Record<NamedReference, readonly string[]>>;

export const NO_REFERENCES: ReferencedIds = {
  company: [],
  app_user: [],
  team: [],
  stage: [],
  pipeline: [],
  project: [],
  product: [],
};

/**
 * The ids the clauses of these trees hold for a field that points at a record
 * a sentence can name. A caller hands them to `useSentenceWords`, which reads
 * the names; nothing else walks a tree for them.
 */
export function referencedIdsOf(
  trees: readonly Node[],
  fields: readonly VocabularyField[] | undefined,
): ReferencedIds {
  const clauses: ClauseRef[] = [];
  const walk = (node: Node) => {
    if (isGroup(node)) {
      node.children.forEach(walk);
      return;
    }
    clauses.push({ field: node.field, operand: node.value });
  };
  trees.forEach(walk);
  return referencedIdsOfClauses(clauses, fields);
}

/** The same ids for loose clauses, such as the ones a Live List verdict states. */
export function referencedIdsOfClauses(
  clauses: readonly ClauseRef[],
  fields: readonly VocabularyField[] | undefined,
): ReferencedIds {
  const found = new Map<string, Set<string>>();
  for (const { field, operand } of clauses) {
    const kind = fields?.find((known) => known.name === field)?.references;
    if (!isNamedReference(kind)) {
      continue;
    }
    const ids = found.get(kind) ?? new Set<string>();
    for (const id of Array.isArray(operand) ? operand : [operand]) {
      if (typeof id === "string" && id !== "") {
        ids.add(id);
      }
    }
    found.set(kind, ids);
  }
  const idsOf = (kind: NamedReference) => [...(found.get(kind) ?? [])];
  return {
    company: idsOf("company"),
    app_user: idsOf("app_user"),
    team: idsOf("team"),
    stage: idsOf("stage"),
    pipeline: idsOf("pipeline"),
    project: idsOf("project"),
    product: idsOf("product"),
  };
}

/** The ids of several walks as one, each id once. */
export function mergeReferencedIds(
  parts: readonly ReferencedIds[],
): ReferencedIds {
  const all = (kind: NamedReference) => [
    ...new Set(parts.flatMap((part) => part[kind])),
  ];
  return {
    company: all("company"),
    app_user: all("app_user"),
    team: all("team"),
    stage: all("stage"),
    pipeline: all("pipeline"),
    project: all("project"),
    product: all("product"),
  };
}

function optionNames(
  options: readonly ReferenceOption[],
  ids: readonly string[],
): [string, string][] {
  return options
    .filter((option) => ids.includes(option.value))
    .map((option): [string, string] => [option.value, option.label]);
}

/**
 * The names of the records the clauses point at. Companies and colleagues are
 * read by id. The rest come from the catalogues the filter pickers read.
 */
export function useRecordNames(
  refs: ReferencedIds,
): ReadonlyMap<string, string> {
  const companies = useQueries({
    queries: refs.company.map((id) => entityNameObserverQuery("company", id)),
  });
  const members = useMemberNames(refs.app_user);
  const teams = useReferenceOptions("team", refs.team.length > 0);
  const stages = useReferenceOptions("stage", refs.stage.length > 0);
  const pipelines = useReferenceOptions("pipeline", refs.pipeline.length > 0);
  const projects = useReferenceOptions("project", refs.project.length > 0);
  const products = useReferenceOptions("product", refs.product.length > 0);
  const names = new Map(members);
  refs.company.forEach((id, index) => {
    const name = companies[index]?.data;
    if (name) {
      names.set(id, name);
    }
  });
  const catalogues: [readonly ReferenceOption[], readonly string[]][] = [
    [teams.options, refs.team],
    [stages.options, refs.stage],
    [pipelines.options, refs.pipeline],
    [projects.options, refs.project],
    [products.options, refs.product],
  ];
  for (const [options, ids] of catalogues) {
    for (const [id, name] of optionNames(options, ids)) {
      names.set(id, name);
    }
  }
  return names;
}

/**
 * The names of the companies, colleagues or teams a clause points at.
 * Undefined when any has no name yet, so the sentence counts them all.
 */
export function recordNamesText(
  operand: unknown,
  field: VocabularyField,
  recordNames: ReadonlyMap<string, string> | undefined,
): string | undefined {
  if (!isNamedReference(field.references) || recordNames === undefined) {
    return undefined;
  }
  const refs: unknown[] = Array.isArray(operand) ? operand : [operand];
  const names = refs.map((id) =>
    typeof id === "string" ? recordNames.get(id) : undefined,
  );
  return names.every((name) => name !== undefined)
    ? names.join(", ")
    : undefined;
}

/**
 * The names of a tag clause's tags, or undefined when any tag has no name.
 * The catalog may be withheld from this seat, or lack the tag. The caller then
 * counts the tags instead. A retired tag keeps its own wording.
 */
export function tagNamesText(
  operand: unknown,
  field: VocabularyField,
  tagNames: ReadonlyMap<string, string> | undefined,
  hasRetiredTag: boolean,
): string | undefined {
  if (field.references !== "tag" || tagNames === undefined || hasRetiredTag) {
    return undefined;
  }
  const tagIds: unknown[] = Array.isArray(operand) ? operand : [operand];
  const names = tagIds.map((id) =>
    typeof id === "string" ? tagNames.get(id) : undefined,
  );
  return names.every((name) => name !== undefined)
    ? names.map((name) => `"${name}"`).join(", ")
    : undefined;
}
