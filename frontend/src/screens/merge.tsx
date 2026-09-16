// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import { navigate, type Route } from "../app/router";
import { Button, Modal, SearchField } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import "./candidatepicker.css";

// The shared "Merge into…" affordance (P-2): a human direct call that folds
// this record (the source, A) into a picked survivor (B) — A is archived
// with merged_into_id=B, B keeps the id the rest of the CRM already points
// at. Contact and Company 360s have an identical merge shape (target_id body
// + If-Match precondition, survivor Contact/Company back), so this stays
// resource-agnostic: the screen supplies the search transport, the merge
// transport, and where the survivor's 360 lives.

const SEARCH_DEBOUNCE_MS = 250;

type MergeCandidate = { id: string; name: string };

export function MergeAction<Survivor extends { id: string }>({
  label,
  sourceId,
  sourceName,
  searchTargets,
  merge,
  invalidate,
  recordKey,
  survivorRoute,
  disabledReasonId,
}: Readonly<{
  label: string;
  sourceId: string;
  sourceName: string;
  // Excludes sourceId from its result — the source row is never a valid
  // merge target for itself.
  searchTargets: (q: string) => Promise<MergeCandidate[]>;
  // POSTs the merge; the screen attaches ifMatch(sourceVersion) itself, so
  // this stays agnostic of the source record's shape. Returns the surviving
  // record.
  merge: (targetId: string) => Promise<Survivor>;
  invalidate: string;
  recordKey: string;
  survivorRoute: (targetId: string) => Route;
  // Why this merge is unavailable, when it is: the id of an element on the
  // page already carrying the sentence. STATE-4a settles the absent-vs-
  // disabled question by CAUSE — a merge blocked by the source record's
  // STATE, an archived row the server will not fold into anything, stays
  // visible and disabled WITH the reason, because the reason is the
  // information and hiding the control hides a fact the reader needs.
  disabledReasonId?: string;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const headingId = useId();
  const [open, setOpen] = useState(false);
  const [term, setTerm] = useState("");
  const [candidates, setCandidates] = useState<MergeCandidate[]>([]);
  const [target, setTarget] = useState<MergeCandidate | null>(null);
  // The caught failure itself, not a sentence about it: the effect below
  // runs debounced and must not depend on the translator, which is a new
  // function every render. It is turned into copy where it is rendered.
  const [searchFailure, setSearchFailure] = useState<unknown>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    const query = term.trim();
    if (!query) {
      setCandidates([]);
      setSearchFailure(null);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(async () => {
      try {
        const results = await searchTargets(query);
        if (!cancelled) {
          setCandidates(
            results.filter((candidate) => candidate.id !== sourceId),
          );
          setSearchFailure(null);
        }
      } catch (error) {
        if (!cancelled) {
          setCandidates([]);
          setSearchFailure(error);
        }
      }
    }, SEARCH_DEBOUNCE_MS);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [open, term, sourceId, searchTargets]);

  const mutation = useMutation({
    mutationFn: (targetId: string) => merge(targetId),
    onSuccess: (survivor) => {
      queryClient.invalidateQueries({ queryKey: [invalidate] });
      queryClient.invalidateQueries({ queryKey: [recordKey, sourceId] });
      queryClient.invalidateQueries({ queryKey: [recordKey, survivor.id] });
      setOpen(false);
      navigate(survivorRoute(survivor.id));
    },
  });

  const close = () => {
    setOpen(false);
    setTerm("");
    setCandidates([]);
    setTarget(null);
    setSearchFailure(null);
    mutation.reset();
  };

  return (
    <>
      <Button
        reasonId={disabledReasonId}
        onClick={() => setOpen(true)}
        data-testid="merge-record"
      >
        {label}
      </Button>
      <Modal open={open} onClose={close} labelledBy={headingId}>
        <Heading
          size="large"
          id={headingId}
          className="t-h2"
          style={{ marginBottom: "var(--space-3)" }}
        >
          {label}
        </Heading>
        <p className="t-caption" style={{ marginBottom: 8 }}>
          {t("merge.pickTarget")}
        </p>
        <SearchField
          placeholder={t("merge.searchPlaceholder")}
          aria-label={t("merge.searchPlaceholder")}
          value={term}
          onChange={(event) => {
            setTerm(event.target.value);
            setTarget(null);
          }}
        />
        {searchFailure ? (
          <p className="t-caption" style={{ color: "var(--dangerText)" }}>
            {problemMessageOf(searchFailure, t)}
          </p>
        ) : null}
        <ul style={{ listStyle: "none", margin: "8px 0", padding: 0 }}>
          {candidates.map((candidate) => (
            <li key={candidate.id}>
              <Button
                className="candidate-option"
                aria-pressed={target?.id === candidate.id}
                onClick={() => setTarget(candidate)}
              >
                {candidate.name}
              </Button>
            </li>
          ))}
        </ul>
        {target && (
          <p style={{ marginBottom: 16 }}>
            {t("merge.confirm", { source: sourceName, target: target.name })}
          </p>
        )}
        {mutation.isError && (
          <p className="t-caption" style={{ color: "var(--dangerText)" }}>
            {problemMessageOf(mutation.error, t)}
          </p>
        )}
        <div className="actions">
          <Button onClick={close} disabled={mutation.isPending}>
            {t("create.cancel")}
          </Button>
          <Button
            variant="danger"
            disabled={!target || mutation.isPending}
            onClick={() => {
              if (target) {
                mutation.mutate(target.id);
              }
            }}
            data-testid="merge-confirm"
          >
            {t("merge.submit")}
          </Button>
        </div>
      </Modal>
    </>
  );
}

export type { MergeCandidate };
