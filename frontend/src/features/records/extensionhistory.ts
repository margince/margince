import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { TimelineEntry } from "../../design-system/timelineentry";
import type { EntityKind } from "../../lib/entity";
import { unwrap } from "../../lib/problem";
import { recordKindLabel } from "../../lib/recordkinds";

type ExtensionHistoryEntry = components["schemas"]["ExtensionHistoryEntry"];

// The record pages the server reads extension entries for. A lead or a project
// has none, so asking would only fetch an empty answer.
function hasExtensionHistory(
  kind: EntityKind,
): kind is "contact" | "company" | "deal" {
  return kind === "contact" || kind === "company" || kind === "deal";
}

/**
 * useExtensionHistory reads what installed extensions add to one record's
 * History: rows of their own kinds that reference it, and their signals. The
 * core builds each entry, so no extension draws one itself.
 */
export function useExtensionHistory(
  kind: EntityKind,
  recordId: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ["extension-history", kind, recordId],
    enabled: enabled && hasExtensionHistory(kind),
    queryFn: async () => {
      if (!hasExtensionHistory(kind)) {
        return { data: [] };
      }
      return unwrap(
        await api.GET("/extension-history", {
          params: { query: { entity_type: kind, entity_id: recordId } },
        }),
      );
    },
  });
}

/**
 * extensionTimeline draws the entries as timeline rows. The kind names itself
 * beside the title, so a reader tells a visit from a payment signal, and a kind
 * the build has no text for still reads as its own name.
 */
export function extensionTimeline(
  entries: readonly ExtensionHistoryEntry[],
): TimelineEntry[] {
  return entries.map((entry) => ({
    // The source is part of the id: a row and a signal can share a uuid space
    // only by accident, and the list keys on this.
    id: `${entry.source === "record" ? "extension" : "signal"}:${entry.id}`,
    kind: entry.source === "record" ? "extension" : "signal",
    title: entry.title,
    qualifier: recordKindLabel(entry.kind, []),
    atIso: entry.at,
    atPrecision: "instant",
    provenance: { kind: "system" },
  }));
}
