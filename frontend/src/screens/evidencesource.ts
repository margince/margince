import type { EvidenceMarkSource } from "../design-system/evidencemark";
import { confidenceLevel } from "../design-system/trust";
import { formatDateTime } from "../format/format";
import type { Locale } from "../i18n";
import { provenanceOf } from "./common";

/**
 * A row carrying its own provenance: a profile field, a fact, a technical
 * signal. The shape is the contract's, minus everything the mark does not read.
 */
export type EvidenceBearingRow = Readonly<{
  captured_by?: string;
  confidence?: number | null;
  evidence_snippet?: string | null;
  source_url?: string | null;
  updated_at?: string;
  /**
   * When the SOURCE was last read, as distinct from when we first recorded the
   * claim. A technical signal is the case that needs it: a mail provider read
   * from DNS this morning and one read a month ago are the same claim with the
   * same updated_at if nothing changed, and only this tells them apart.
   */
  retrieved_at?: string | null;
}>;

/**
 * derivedSource builds the evidence mark's payload for a value the system read
 * rather than a person typed.
 *
 * A value a HUMAN entered gets no mark: the record is full of human-entered
 * values, and marking them all would make the underline mean nothing.
 *
 * Shared rather than per-screen so the "confidence is never hidden" convention
 * cannot drift between the surfaces that show provenance — the profile fields,
 * the evidence card and the technical profile all answer "how do you know?"
 * the same way.
 */
export function derivedSource(
  row: EvidenceBearingRow,
  locale: Locale,
  recordZone: string,
): EvidenceMarkSource | undefined {
  const provenance = provenanceOf(row.captured_by);
  if (provenance.kind === "human") {
    return undefined;
  }
  // When the source was read is the better answer to "is this still true?", so
  // it wins where a row carries one. Falling back to updated_at keeps every
  // existing row reading exactly as it did.
  const observed = row.retrieved_at ?? row.updated_at;
  return {
    provenance,
    confidence: confidenceLevel(row.confidence) ?? undefined,
    snippet: row.evidence_snippet,
    sourceUrl: row.source_url,
    at: observed ? formatDateTime(observed, locale, recordZone) : undefined,
  };
}
