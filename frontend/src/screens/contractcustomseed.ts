// Seeding a form's CUSTOM half from a catalog that arrives on its own schedule.
//
// The core fields of a record form are seeded once, from the record. The custom
// fields cannot be: their catalog is a second read that can land after the form
// opens, change while it stays open, and be read against a different record than
// the one the form opened on. Three arrivals, and what a reader has already
// typed has to survive all of them.
//
// Its own module because contractform.tsx is at its length ceiling and this is
// the part of it with no JSX in it at all.

import { useEffect, useRef } from "react";
import type { CreateField } from "./create";
import { prefillFromRecord, seedMissingFields } from "./edit.prefill";

export type CustomFieldSeed = Readonly<{
  open: boolean;
  /** The record the form is editing, or null while recording a new one. */
  recordId: string | null;
  /**
   * The record's custom slice, RAW.
   *
   * Raw because the write diffs the form against this baseline and converts it
   * itself — handing it an already-converted value converts a currency twice: a
   * stored 10000 reads as "100", re-converts to "1", and a genuine edit to 1
   * then compares EQUAL and disappears without a trace.
   */
  stored: Record<string, unknown>;
  formFields: CreateField[];
  /**
   * The catalog this draft is filled from, by column NAMES.
   *
   * Names and not a count: a catalog that swaps one field for another keeps the
   * same count, which would leave the draft holding a retired field's value
   * while the controls draw the new one.
   */
  catalogKey: string;
  /** What the controls hold now. */
  values: Record<string, string>;
  /** A different record: the whole custom half belongs to it. */
  replace: (
    values: Record<string, string>,
    baseline: Record<string, unknown>,
  ) => void;
  /** The same record, more fields: only the missing ones are seeded. */
  add: (
    values: Record<string, string>,
    baseline: Record<string, unknown>,
  ) => void;
}>;

/**
 * Keeps a form's custom values in step with the catalog, without overwriting
 * what the reader has typed.
 *
 * TWO events reach here and they take OPPOSITE answers, and conflating them is
 * what a hand-rolled version of this got wrong in both directions. A different
 * record replaces the custom half outright; a catalog that grew under the same
 * record seeds only what is missing, because everything already in there is
 * what the reader has been typing. Keyed on the catalog alone, a form handed a
 * different record showed blanks over its stored values; reseeding wholesale on
 * a catalog change threw an in-progress edit away.
 */
export function useSeededCustomFields(seed: CustomFieldSeed): void {
  const seededRecord = useRef<string | null>(null);
  const seededCatalog = useRef<string | null>(null);
  // The whole argument, readable from the effect WITHOUT the effect depending
  // on it: `values` changes on every keystroke and `stored` is a fresh object
  // every render, so either in the dependency list would re-run this constantly.
  const latest = useRef(seed);
  latest.current = seed;
  const { open, recordId, catalogKey } = seed;

  useEffect(() => {
    if (!open) {
      seededRecord.current = null;
      seededCatalog.current = null;
      return;
    }
    const current = latest.current;
    if (seededRecord.current !== recordId) {
      seededRecord.current = recordId;
      seededCatalog.current = catalogKey;
      current.replace(
        prefillFromRecord(current.formFields, current.stored),
        current.stored,
      );
      return;
    }
    if (seededCatalog.current === catalogKey) {
      return;
    }
    seededCatalog.current = catalogKey;
    const seeded = seedMissingFields(
      current.formFields,
      current.stored,
      current.values,
    );
    if (!seeded) {
      return;
    }
    // The baseline grows with the form. A field the opening reading never
    // carried would otherwise make a genuine clear compare equal to it and
    // vanish from the patch — see seedMissingFields.
    current.add(seeded.form, seeded.raw);
  }, [open, recordId, catalogKey]);
}
