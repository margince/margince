import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import {
  Badge,
  Button,
  Disclosure,
  Field,
  Textarea,
} from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { isMessageKey, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  leadManualSignalsKey,
  leadSignalCatalogKey,
  leadWriteKeys,
} from "../../lib/leadkeys";
import { unwrap } from "../../lib/problem";
import "./leadsignals.css";

type SetSignalRequest = components["schemas"]["SetLeadManualSignalRequest"];
type SignalFactor = SetSignalRequest["factor"];
type SignalKind = SetSignalRequest["signal_kind"];
type ManualSignal = components["schemas"]["LeadManualSignal"];

type SignalCatalogFactor = components["schemas"]["LeadSignalCatalogFactor"];

/**
 * The factors a human may supply and the bands each accepts, as the server
 * validates them. Read from `/lead-signal-catalog`, so the form offers
 * what the store will take and the band list is spelled in one place.
 */
export function useLeadSignalCatalog() {
  return useQuery({
    queryKey: leadSignalCatalogKey,
    staleTime: 60 * 60 * 1000,
    queryFn: async () => unwrap(await api.GET("/lead-signal-catalog")).factors,
  });
}

/**
 * The factors to list. The catalog is authoritative while it answers.
 * Without it (a client newer than its server), the stored factors are listed
 * so a recorded signal is never hidden.
 */
export function listedFactors(
  catalog: readonly SignalCatalogFactor[] | undefined,
  stored: readonly ManualSignal[],
): readonly SetSignalRequest["factor"][] {
  if (catalog) {
    return catalog.map(({ factor }) => factor);
  }
  return [...new Set(stored.map(({ factor }) => factor))];
}

/** A translated signal label, or the stored value as written when none exists. */
export function signalLabel(
  t: ReturnType<typeof useT>,
  key: string,
  raw: string,
): string {
  const full = `lead.signal.${key}`;
  return isMessageKey(full) ? t(full) : raw;
}

const SIGNAL_KINDS: readonly SignalKind[] = ["fact", "assumption", "judgement"];

const CONFIDENCE_LEVELS = ["0.5", "0.7", "0.9", "1"] as const;

/**
 * What the wire carries when the rep never opens "More".
 *
 * `assumption`, not `fact`: the three questions ask what a rep believes about
 * an account, not what they can show. Recording an unqualified answer as a
 * verified fact would put a claim on the score that nobody made, and
 * `assumption` ("a working estimate") is the weakest thing the contract's
 * `LeadManualSignalKind` can say — the enum has no "unstated", so this is what
 * an unopened disclosure honestly means.
 *
 * Confidence stays NULL, which the contract allows and which is the only value
 * that claims nothing. Any number — 0.5 included — is a statement about
 * certainty, and a rep who was never asked made none. Points come from the
 * band alone (`leadManualFactorBands`), so a null confidence neither flatters
 * nor penalises the score.
 *
 * Change either default and every signal entered afterwards means something
 * different from the ones already stored, with nothing on the row to say so.
 */
const DEFAULT_SIGNAL_KIND: SignalKind = "assumption";
const CONFIDENCE_UNSTATED = "";

/**
 * What a stored signal holds once its stock sentence was taken away. The
 * server writes it (contacts.ManualSignalNoEvidence) and refuses it as input,
 * so the screen shows a translated label instead of the English constant.
 */
export const MANUAL_SIGNAL_NO_EVIDENCE = "No evidence recorded";

/** The evidence line of a stored signal, with the marker translated. */
export function evidenceText(
  reason: string,
  t: (key: MessageKey) => string,
): string {
  return reason === MANUAL_SIGNAL_NO_EVIDENCE
    ? t("lead.signalNoEvidence")
    : reason;
}

// One band per factor, absent for a question the rep left alone. Only answered
// questions are written: filling these in from what is already stored would
// re-stamp `set_by`/`set_at` on factors nobody touched.
type Answers = Readonly<Partial<Record<SignalFactor, string>>>;
const NO_ANSWERS: Answers = {};

/** Every manual signal on the lead, live and superseded. */
export function useManualSignals(id: string) {
  return useQuery({
    queryKey: leadManualSignalsKey(id),
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/leads/{id}/manual-signals", {
          params: { path: { id } },
        }),
      );
      return data.data;
    },
  });
}

/**
 * LeadManualSignals is the human half of the score (S-E13.6, ADR-0105 §4):
 * a rep enters what capture cannot fetch — a traffic band, an employee
 * count, a budget hint — and it feeds the same transparent score, showing up
 * in the decomposition as its own factor, never blended into an
 * auto-captured one.
 *
 * The form asks the three plain questions and nothing else. Evidence quality
 * and confidence still reach the wire, because they are what makes a manual
 * signal auditable, but they sit behind "More" with the defaults above: a
 * form that opens by asking a rep to grade their own evidence is a form a rep
 * abandons.
 *
 * Read-only on a terminal lead, WITH the reason: the inputs a rep made are
 * part of why the lead was worked, and hiding them on a closed lead would
 * hide the fact the reader came for (STATE-4a).
 */
export function LeadManualSignals({
  id,
  readOnlyReason,
  onSaved,
}: Readonly<{ id: string; readOnlyReason?: string; onSaved?: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const queryClient = useQueryClient();
  const signals = useManualSignals(id);
  const catalog = useLeadSignalCatalog();
  const factors: readonly SignalCatalogFactor[] = catalog.data ?? [];
  const [answers, setAnswers] = useState<Answers>(NO_ANSWERS);
  const [note, setNote] = useState("");
  const [kind, setKind] = useState<SignalKind>(DEFAULT_SIGNAL_KIND);
  const [confidence, setConfidence] = useState<string>(CONFIDENCE_UNSTATED);

  const invalidate = () => {
    // leadWriteKeys carries `["lead", id]`, and React Query invalidates by
    // PREFIX — so the manual-signals read under `["lead", id, …]` is already
    // reached. Naming it again is a second refetch of the same rows.
    for (const key of leadWriteKeys(id)) {
      queryClient.invalidateQueries({ queryKey: key });
    }
  };
  const set = useMutation({
    // The batch arrives as the mutation's variable, never read back out of
    // render state (mutation-variable-coverage.test.ts): the click that
    // carries it belongs to the render the rep was looking at.
    mutationFn: async (writes: readonly SetSignalRequest[]) => {
      // Sequential on purpose. Each PUT recomputes the lead's score inside
      // its own transaction, so three in flight at once would race that
      // recompute and the last total written could be missing the others'
      // points.
      for (const body of writes) {
        unwrap(
          await api.PUT("/leads/{id}/manual-signals", {
            params: { path: { id } },
            body,
          }),
          t,
        );
        // Retired as it lands, not once the whole batch has. A batch can stop
        // part-way through, and an answer still on the form after the server
        // took it is one the next save re-sends: that re-stamps
        // `set_by`/`set_at` on a factor nobody touched the second time and
        // appends a history row recording no change. What stays on the form
        // is exactly what is still outstanding.
        setAnswers((prev) => ({ ...prev, [body.factor]: undefined }));
      }
    },
    onSuccess: () => {
      setNote("");
      setKind(DEFAULT_SIGNAL_KIND);
      setConfidence(CONFIDENCE_UNSTATED);
      onSaved?.();
    },
    // On failure too: a batch can stop part-way through, and the list above is
    // then the only place the rep can see which answers actually landed.
    onSettled: invalidate,
  });
  const clear = useMutation({
    mutationFn: async (target: SignalFactor) => {
      unwrap(
        await api.DELETE("/leads/{id}/manual-signals/{factor}", {
          params: { path: { id, factor: target } },
        }),
        t,
      );
    },
    onSuccess: invalidate,
  });
  const busy = set.isPending || clear.isPending;
  const canEdit = !readOnlyReason && !busy;
  const answered = factors.filter((entry) => !!answers[entry.factor]);
  const label = (key: string, raw = key) => signalLabel(t, key, raw);

  // `reason` is what the rep typed and nothing else: the store refuses a
  // blank one, so the form never invents a sentence to get past it.
  const writes = (): readonly SetSignalRequest[] =>
    answered.map((entry) => ({
      factor: entry.factor,
      band: answers[entry.factor] ?? "",
      signal_kind: kind,
      confidence:
        confidence === CONFIDENCE_UNSTATED ? null : Number(confidence),
      reason: note.trim(),
    }));

  return (
    <div className="form-stack">
      {/* An absent factor list is not an empty one: while the explanation is
          loading, failed, or not yet retained (ADR-0105 §1), nothing here can
          say what is set, so nothing here claims "not entered". */}
      {signals.isPending && <span>{t("lead.scoreLoading")}</span>}
      <ErrorLine error={signals.error ?? catalog.error} />
      {signals.isSuccess && (catalog.isSuccess || catalog.isError) && (
        <ul className="leadsignals-list">
          {listedFactors(catalog.data, signals.data).map((name) => {
            const entries = signals.data.filter(
              (entry: ManualSignal) => entry.factor === name,
            );
            const live = entries.find((entry) => !entry.superseded_at);
            const superseded = entries.filter((entry) => entry.superseded_at);
            return (
              <li key={name} className="leadsignals-factor">
                <span>{label(name)}</span>
                {live ? (
                  <>
                    <strong>{label(`${name}.${live.band}`, live.band)}</strong>
                    <Badge>
                      {live.points >= 0 ? "+" : ""}
                      {formatNumber(live.points, locale)}
                    </Badge>
                    <span className="t-caption">{label(live.signal_kind)}</span>
                    {live.confidence != null && (
                      <span className="t-caption">
                        {t("lead.signalConfidenceValue", {
                          value: formatNumber(
                            Math.round(live.confidence * 100),
                            locale,
                          ),
                        })}
                      </span>
                    )}
                    <span className="t-caption">
                      {evidenceText(live.reason, t)}
                    </span>
                    <span className="t-caption">
                      <EntityRef kind="user" id={live.set_by} />
                    </span>
                    <span className="t-caption">
                      {t("lead.signalRecordedAt", {
                        at: formatDateTime(live.set_at, locale, zone),
                      })}
                    </span>
                    {!readOnlyReason && (
                      <Button
                        disabled={busy}
                        onClick={() => clear.mutate(name)}
                      >
                        {t("lead.signalClear")}
                      </Button>
                    )}
                  </>
                ) : (
                  <span>{t("lead.signalUnset")}</span>
                )}
                {superseded.map((entry) => (
                  <span
                    className="t-caption"
                    key={`${entry.set_at}-${entry.band}`}
                  >
                    {t("lead.signalSuperseded", {
                      value: label(`${name}.${entry.band}`, entry.band),
                      source:
                        entry.superseded_by ?? t("lead.signalAutomaticSource"),
                    })}
                  </span>
                ))}
              </li>
            );
          })}
        </ul>
      )}
      {readOnlyReason ? (
        <span>{readOnlyReason}</span>
      ) : (
        <div className="form-stack">
          {factors.map(({ factor: name, bands }) => (
            <Field key={name} label={label(`ask.${name}`)}>
              {(control) => (
                <Select
                  {...control}
                  value={answers[name] ?? ""}
                  disabled={!canEdit}
                  placeholder={t("lead.signalBandPick")}
                  onChange={(next) =>
                    setAnswers((prev) => ({ ...prev, [name]: next }))
                  }
                  options={bands.map(({ id: value }) => ({
                    value,
                    label: label(`${name}.${value}`),
                  }))}
                />
              )}
            </Field>
          ))}
          <Field
            label={t("lead.signalReason")}
            hint={t("lead.signalReasonHint")}
            required
          >
            {(control) => (
              <Textarea
                {...control}
                value={note}
                disabled={!canEdit}
                onChange={(event) => setNote(event.target.value)}
              />
            )}
          </Field>
          <Disclosure summary={t("lead.signalMore")}>
            <div className="form-stack">
              <p className="t-caption">{t("lead.signalProvenanceHint")}</p>
              <Field label={t("lead.signalEvidenceQuality")}>
                {(control) => (
                  <Select
                    {...control}
                    value={kind}
                    disabled={!canEdit}
                    onChange={(next) => {
                      // `find` rather than an assertion: the Select reports a
                      // string, and only a value the catalog actually holds
                      // may become the kind on the wire.
                      const picked = SIGNAL_KINDS.find(
                        (value) => value === next,
                      );
                      if (picked) {
                        setKind(picked);
                      }
                    }}
                    options={SIGNAL_KINDS.map((value) => ({
                      value,
                      label: label(value),
                    }))}
                  />
                )}
              </Field>
              <Field label={t("lead.signalConfidence")}>
                {(control) => (
                  <Select
                    {...control}
                    value={confidence}
                    disabled={!canEdit}
                    placeholder={t("lead.signalConfidenceUnstated")}
                    onChange={setConfidence}
                    options={[
                      {
                        value: CONFIDENCE_UNSTATED,
                        label: t("lead.signalConfidenceUnstated"),
                      },
                      ...CONFIDENCE_LEVELS.map((value) => ({
                        value,
                        label: t("lead.signalConfidenceValue", {
                          value: formatNumber(
                            Math.round(Number(value) * 100),
                            locale,
                          ),
                        }),
                      })),
                    ]}
                  />
                )}
              </Field>
            </div>
          </Disclosure>
          <ErrorLine error={set.isError ? set.error : clear.error} />
          <div className="form-actions">
            <Button
              variant="primary"
              disabled={!canEdit || answered.length === 0 || note.trim() === ""}
              onClick={() => set.mutate(writes())}
            >
              {t("lead.signalSave")}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
