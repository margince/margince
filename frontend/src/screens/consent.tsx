import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  Skeleton,
  TextInput,
} from "../design-system/atoms";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { humanizeToken } from "./audit";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import "./consent.css";
import { stable } from "../format/collate";

// The Art. 7 proof log (G-4) + the double-opt-in redeem field (G-5) for the
// Person 360. GET /people/{id}/consent already returns {state, events}; this
// is the only surface that reads events — the 360 previously rendered state
// alone and silently dropped the append-only trail. requires_double_opt_in
// lives on ConsentPurpose, not on the person's per-purpose state, so this
// section also reads GET /consent-purposes and joins on purpose_id to know
// which rows need a token before a grant can take effect.

type ConsentPurpose = components["schemas"]["ConsentPurpose"];
type PersonConsentState = components["schemas"]["PersonConsentState"];
type ConsentEvent = components["schemas"]["ConsentEvent"];

function usePersonConsent(personId: string) {
  return useQuery({
    queryKey: ["person-consent", personId],
    queryFn: async () => {
      const { data, error } = await api.GET("/people/{id}/consent", {
        params: { path: { id: personId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// Same cache key settings.tsx's ConsentPurposesCard uses, so the two
// surfaces share one fetch. No pagination — the endpoint hardcodes
// has_more:false, so there is no second page to walk.
export function useConsentPurposes() {
  return useQuery({
    queryKey: ["consent-purposes"],
    queryFn: async () => {
      const { data, error } = await api.GET("/consent-purposes");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// The label for each actor kind the wire can name, keyed on the closed
// actor_type enum (audit.tsx's ACTOR_ICON is the same table shape). Keying on
// the union makes a kind added upstream a compile error here rather than a
// silent mislabel — a proof log must never assert an actor the wire did not.
const ACTOR_LABEL: Record<
  NonNullable<ConsentEvent["actor_type"]>,
  MessageKey
> = {
  human: "consent.actorHuman",
  agent: "consent.actorAgent",
  system: "consent.actorSystem",
  connector: "consent.actorConnector",
};

// The actor line on the Art. 7 proof log: names WHO the server says captured
// this decision, verbatim. This is deliberately NOT ProvenanceTag — that
// component exists for a compose/staging context ("did *you* type this, or
// an agent") and its human branch renders "typed by you" unconditionally,
// which would misattribute every human-captured grant to the viewer instead
// of the actual actor (frequently a different teammate, or the subject
// themself via a public form). A proof log's actor is evidence, not a claim
// about who is looking at it — it always shows actor_type + actor_id
// straight from the wire, never resolved against the current session.
function ConsentEventActor({ event }: Readonly<{ event: ConsentEvent }>) {
  const t = useT();
  // An event the wire never attributed is unrecorded, never a positive claim
  // about who is looking at it.
  if (!event.actor_type) {
    return <span className="t-caption">{t("consent.actorUnknown")}</span>;
  }
  const label = t(ACTOR_LABEL[event.actor_type]);
  return (
    <span className="t-caption">
      {label}
      {event.actor_id && (
        <>
          {" "}
          <span className="t-mono">{event.actor_id}</span>
        </>
      )}
    </span>
  );
}

// The per-purpose Art. 7 proof log: newest first, one row per transition.
// Never renders policy_text/policy_version — wireEvent never projects them
// (they're NOT NULL in consent_event but genuinely absent on the wire), so
// showing wording here would be fabricated evidence on a GDPR proof surface.
function ConsentProofLog({ events }: Readonly<{ events: ConsentEvent[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  if (events.length === 0) {
    return <EmptyState>{t("consent.proofEmpty")}</EmptyState>;
  }
  const ordered = [...events].sort((a, b) =>
    stable(b.occurred_at, a.occurred_at),
  );
  return (
    <Card as="div" inset className="consent-proof-log">
      {/* `timeline-plain`: these rows carry their date inline rather than in
          the chronicle's gutter, so they opt out of its column grid. */}
      <ul className="timeline timeline-plain">
        {ordered.map((event) => (
          <li key={event.id}>
            <span className="tl-body">
              <span className="tl-title">
                <Badge
                  tone={event.new_state === "granted" ? "success" : "warn"}
                >
                  {humanizeToken(event.new_state)}
                </Badge>{" "}
                {event.source ?? t("consent.sourceUnknown")}
              </span>
              <span className="tl-meta">
                <ConsentEventActor event={event} />
                <span>{formatDateTime(event.occurred_at, locale, zone)}</span>
              </span>
            </span>
          </li>
        ))}
      </ul>
    </Card>
  );
}

// The state badge tone, keyed on the closed consent-state enum: unknown gets no
// tone (it isn't a withdrawal — the noRecord subtitle carries that distinction).
// Keying on the union keeps a state added upstream a compile error here rather
// than a silently untoned badge.
const STATE_TONE: Record<
  PersonConsentState["state"],
  "success" | "warn" | undefined
> = {
  granted: "success",
  withdrawn: "warn",
  unknown: undefined,
};

// A mutation's own refusal, in the server's words rather than a generic
// failure — a DOI-required purpose 422s here, and the human needs to see
// exactly why the toggle didn't take.
function MutationError({ error }: Readonly<{ error: unknown }>) {
  const t = useT();
  if (!error) {
    return null;
  }
  return (
    <p className="t-caption" style={{ color: "var(--danger)" }}>
      {problemMessageOf(error, t)}
    </p>
  );
}

// One consent-purpose row on the Person 360 (P-8/P-9): the state badge, a
// Grant/Withdraw toggle that writes an append-only consent_event through
// POST /people/{id}/consent, the token field a DOI purpose needs before a
// grant takes effect, and a toggleable proof log. lawful_basis is
// intentionally omitted from the toggle body — it's optional in
// RecordConsentRequest and this control has no field for it yet. Errors
// surface verbatim (a DOI-required purpose 422s here rather than silently
// no-opping) so the human sees exactly why the toggle didn't take.
function ConsentRow({
  personId,
  entry,
  purpose,
  events,
}: Readonly<{
  personId: string;
  entry: PersonConsentState;
  purpose: ConsentPurpose | undefined;
  events: ConsentEvent[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const queryClient = useQueryClient();
  const granted = entry.state === "granted";
  const [token, setToken] = useState("");
  const [showLog, setShowLog] = useState(false);
  const requiresDoi = purpose?.requires_double_opt_in ?? false;

  const setState = useMutation({
    mutationFn: async (newState: "granted" | "withdrawn") => {
      const trimmedToken = token.trim();
      const { data, error } = await api.POST("/people/{id}/consent", {
        params: { path: { id: personId } },
        body: {
          purpose_id: entry.purpose_id,
          new_state: newState,
          ...(trimmedToken ? { double_opt_in_token: trimmedToken } : {}),
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // The write endpoint returns only the updated state row, not the new
    // consent_event — so the proof log can only pick up the transition just
    // made by refetching, not by patching the cache from this response.
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["person-consent", personId],
      });
    },
  });

  const issueDoi = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST(
        "/people/{id}/consent/double-opt-in",
        {
          params: { path: { id: personId } },
          // deliver:true would queue a confirmation email per the contract,
          // but the server has no queue call behind it (doi.go mints only) —
          // asking for delivery here would silently lose the token, so this
          // surface takes ownership of disclosing it below instead.
          body: { purpose_id: entry.purpose_id, deliver: false },
        },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  return (
    <div className="consent-row">
      <div className="consent-row-head">
        <strong>
          {purpose?.label ?? entry.purpose_key ?? entry.purpose_id}
        </strong>
        <Badge tone={STATE_TONE[entry.state]}>
          {humanizeToken(entry.state)}
        </Badge>
        {entry.state === "unknown" && (
          <span className="t-caption">{t("consent.noRecord")}</span>
        )}
      </div>
      <div className="consent-row-actions">
        <Button
          small
          disabled={setState.isPending}
          onClick={() => setState.mutate(granted ? "withdrawn" : "granted")}
        >
          {granted ? t("consent.withdraw") : t("consent.grant")}
        </Button>
        {requiresDoi && (
          <Button
            small
            disabled={issueDoi.isPending}
            onClick={() => issueDoi.mutate()}
          >
            {t("consent.doubleOptIn")}
          </Button>
        )}
        <Button small onClick={() => setShowLog((value) => !value)}>
          {t("consent.proofLog")}
        </Button>
      </div>
      {requiresDoi && (
        <div className="field consent-token-field">
          <label className="t-label" htmlFor={`doi-token-${entry.purpose_id}`}>
            {t("consent.tokenLabel")}
          </label>
          <TextInput
            id={`doi-token-${entry.purpose_id}`}
            value={token}
            onChange={(event) => setToken(event.target.value)}
          />
          <p className="t-caption">{t("consent.tokenHint")}</p>
        </div>
      )}
      {setState.isError && <MutationError error={setState.error} />}
      {issueDoi.isError && <MutationError error={issueDoi.error} />}
      {issueDoi.data && (
        <p className="t-caption">
          {t("consent.doiIssued")} <code>{issueDoi.data.token}</code> ·{" "}
          {t("consent.doiExpires")}:{" "}
          {formatDateTime(issueDoi.data.expires_at, locale, zone)}
        </p>
      )}
      {showLog && <ConsentProofLog events={events} />}
    </div>
  );
}

export function ConsentSection({ personId }: Readonly<{ personId: string }>) {
  const t = useT();
  const consentQuery = usePersonConsent(personId);
  const purposesQuery = useConsentPurposes();
  const purposes = purposesQuery.data?.data ?? [];
  // Only trust "no purposes" once the purposes fetch itself has actually
  // succeeded — while it's still pending this would otherwise flash the
  // empty state before the real list ever arrives.
  const noPurposes = purposesQuery.isSuccess && purposes.length === 0;
  const consent = consentQuery.data;

  // requires_double_opt_in lives only on ConsentPurpose, so a row's DOI gate
  // depends on purposesQuery having actually succeeded — a failed fetch that
  // fell back to `[]` here (rather than erroring loudly) would make every
  // DOI-required purpose render as freely grantable, silently dropping a
  // legal control. share.tsx's RosterPicker gates its two roster fetches the
  // same explicit way, for the same reason: a collapsed-to-empty failure
  // must never be mistaken for a real empty list.
  let body: ReactNode = null;
  if (consent) {
    if (purposesQuery.isPending) {
      body = <Skeleton width="60%" />;
    } else if (purposesQuery.isError) {
      body = (
        <EmptyState>
          <p>{t("consent.purposesUnavailable")}</p>
          <Button small onClick={() => purposesQuery.refetch()}>
            {t("common.retry")}
          </Button>
        </EmptyState>
      );
    } else if (noPurposes) {
      body = <EmptyState>{t("consent.noPurposes")}</EmptyState>;
    } else {
      body = (
        <div>
          {consent.state.map((entry) => (
            <ConsentRow
              key={entry.purpose_id}
              personId={personId}
              entry={entry}
              purpose={purposes.find(
                (purpose) => purpose.id === entry.purpose_id,
              )}
              events={consent.events.filter(
                (event) => event.purpose_id === entry.purpose_id,
              )}
            />
          ))}
        </div>
      );
    }
  }

  return (
    <Card
      style={{ marginBottom: "var(--space-4)" }}
      ariaLabel={t("person.consent")}
      title={t("person.consent")}
      sub={t("consent.defaultDeny")}
    >
      <QueryStates query={consentQuery}>{body}</QueryStates>
      {/* Per PERSON rather than per purpose, so it sits under the rows instead
          of inside one: the link opens everything held about them and asks the
          marketing question once, which is not a fact about any single
          purpose. */}
      {/* Keyed on the person: a mutation result is about the record it was
          asked for, and React would otherwise reuse this component across a
          navigation between two cached contacts and leave the previous
          contact's address sitting under the new record. */}
      <ConfirmDetailsAction key={personId} personId={personId} />
    </Card>
  );
}

/** What to say about a link that was just issued. The three outcomes are three
 * different next moves for the reader, so each gets its own sentence. */
function sentenceFor(
  issued: components["schemas"]["ConfirmRequestIssued"],
  t: ReturnType<typeof useT>,
): string {
  const address = issued.delivered_to;
  if (issued.delivered) {
    return t("consent.askSent", { address });
  }
  return issued.sendable
    ? t("consent.askSendFailed", { address })
    : t("consent.askNotDelivered", { address });
}

/**
 * ConfirmDetailsAction mails the contact a link to see what is held about them,
 * correct it, and answer on marketing.
 *
 * The address is never chosen here. The server derives it from the person's own
 * live primary email, which is what lets a grant made through the link stand on
 * its own: the answer came from the subject's mailbox. So this surface offers
 * the act and reports where it went, and cannot aim it anywhere.
 */
function ConfirmDetailsAction({ personId }: Readonly<{ personId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const mayAsk = useCanWrite("person", "update");
  const ask = useMutation({
    // Keyed on the person, so a result belongs to the record it was asked
    // about. React reuses this component across a navigation between two
    // cached contacts, and without the key the previous contact's address sat
    // under the new record's rows — naming somebody else's mailbox as the one
    // this contact's link went to.
    mutationKey: ["confirm-request", personId],
    mutationFn: async (id: string) => {
      const { data, error } = await api.POST(
        "/people/{id}/consent/confirm-request",
        { params: { path: { id } } },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  if (!mayAsk) {
    return null;
  }
  return (
    <div className="consent-confirm-ask">
      <Button
        small
        disabled={ask.isPending}
        data-testid="confirm-details-ask"
        onClick={() => ask.mutate(personId)}
      >
        {t("consent.askToConfirm")}
      </Button>
      <p className="t-caption">{t("consent.askToConfirmWhat")}</p>
      {ask.isError && <MutationError error={ask.error} />}
      {ask.data && (
        <p className="t-caption" data-testid="confirm-details-sent">
          {/* Three outcomes: it went, this installation cannot send at all,
              or the send was tried and failed. The middle and the last ask
              different things of the reader — configure a relay, or press
              again — so they cannot share a sentence. */}
          {sentenceFor(ask.data, t)} {t("consent.askExpires")}:{" "}
          {formatDateTime(ask.data.expires_at, locale, zone)}
        </p>
      )}
    </div>
  );
}
