import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Copy } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, Button, Disclosure, Modal } from "../../design-system/atoms";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { type Fact, FactList } from "../../design-system/factlist";
import { Heading } from "../../design-system/heading";
import { IconAction } from "../../design-system/iconaction";
import { QueryStates } from "../../design-system/querystates";
import { StatusBadge } from "../../design-system/statusbadge";
import {
  formatDateTime,
  formatDecimal,
  formatNumber,
  ordinalNumber,
} from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { providerName } from "../../lib/ai-provider-names";
import { aiTaskLabel } from "../../lib/ai-task-label";
import { unwrap } from "../../lib/problem";
import {
  attemptReasonLabel,
  callCodeName,
  callCodeTone,
  tierLabel,
} from "./ai-decision-labels";
import {
  type AiCallSummary,
  costLabel,
  ladderAttempts,
  OUTCOME_KIND,
  outcomeLabel,
} from "./aicalls-vocab";

export type AiCallRecord = components["schemas"]["AiCall"];
type AiCallAttempt = components["schemas"]["AiCallAttempt"];
type T = ReturnType<typeof useT>;

// A string response is shown verbatim (real newlines); an object is
// pretty-printed. Either way the .code-block surface wraps and scrolls it.
function payloadText(value: unknown): string {
  return typeof value === "string" ? value : JSON.stringify(value, null, 2);
}

// One attempt's model and its vendor's name, the pair the call row shows.
function attemptBinding(attempt: AiCallAttempt, t: Translator): string {
  return [
    attempt.model_id,
    attempt.provider && providerName(attempt.provider, t),
  ]
    .filter(Boolean)
    .join(" · ");
}

const IDENTITY_SOURCE: Readonly<Record<string, MessageKey>> = {
  response: "aicalls.detail.source.response",
  echo: "aicalls.detail.source.echo",
  configured: "aicalls.detail.source.configured",
};

// How far to trust the served model named above. A source this screen does
// not know yet is shown as sent.
function identitySource(source: string, t: Translator): string {
  const key = IDENTITY_SOURCE[source];
  return key ? t(key) : t("aicalls.detail.source", { source });
}

function useCallDetail(id: string) {
  return useQuery({
    queryKey: ["ai-call", id],
    queryFn: async () =>
      unwrap(await api.GET("/ai/calls/{id}", { params: { path: { id } } })),
  });
}

function Section({
  title,
  children,
}: Readonly<{ title: string; children: ReactNode }>) {
  return (
    <section className="aicalls-section">
      <Heading size="xsmall" as="h3">
        {title}
      </Heading>
      {children}
    </section>
  );
}

// The figures the list row already carries, so the drawer opens on them at
// once. Only the attempt ladder and the payload wait on the detail read.
function summaryFacts(
  call: AiCallSummary,
  detail: AiCallRecord | undefined,
  t: T,
  n: (value: number) => string,
  locale: Locale,
): Fact[] {
  const facts: (Fact | false)[] = [
    { key: "model", term: t("aicalls.fact.served"), value: call.served_model },
    {
      key: "configured",
      term: t("aicalls.fact.configured"),
      value: call.model_id || t("aicalls.fact.none"),
    },
    {
      key: "provider",
      term: t("aicalls.fact.provider"),
      value: providerName(call.provider, t),
    },
    !!call.tier && {
      key: "tier",
      term: t("aicalls.fact.tier"),
      value: tierLabel(call.tier, t),
    },
    !!call.error_sentinel && {
      key: "error",
      term: t("aicalls.fact.error"),
      value: callCodeName(call.error_sentinel, t),
    },
    {
      key: "tokens",
      term: t("aicalls.col.tokens"),
      value: t("aicalls.fact.tokensInOut", {
        in: n(call.tokens_in),
        out: n(call.tokens_out),
      }),
    },
    call.reasoning_tokens > 0 && {
      key: "reasoning",
      term: t("aicalls.fact.reasoning"),
      value: n(call.reasoning_tokens),
    },
    call.cached_tokens > 0 && {
      key: "cached",
      term: t("aicalls.fact.cached"),
      value: n(call.cached_tokens),
    },
    {
      key: "cost",
      term: t("aicalls.col.cost"),
      value:
        call.estimated_cost_microusd === undefined
          ? `${costLabel(call, locale, t)}. ${t("aicalls.cost.unpricedHint")}`
          : costLabel(call, locale, t),
    },
    {
      key: "latency",
      term: t("aicalls.col.latency"),
      value: t("aicalls.ms", { value: n(call.latency_ms) }),
    },
    {
      key: "attempts",
      term: t("aicalls.detail.attempts"),
      value: n(ladderAttempts(call)),
    },
    !!detail && {
      key: "source",
      term: t("aicalls.fact.source"),
      value: identitySource(detail.served_identity_source, t),
    },
    !!detail && {
      key: "context",
      term: t("aicalls.fact.context"),
      value:
        detail.context_scopes.length > 0
          ? detail.context_scopes.join(", ")
          : t("aicalls.fact.none"),
    },
  ];
  return facts.filter((fact): fact is Fact => fact !== false);
}

// Exact identifiers for whoever has to quote this call elsewhere.
function technicalFacts(
  call: AiCallSummary,
  detail: AiCallRecord | undefined,
  t: T,
): Fact[] {
  const ids: [string, string, string | null | undefined][] = [
    ["time", t("aicalls.fact.exactTime"), call.occurred_at],
    ["task", t("aicalls.fact.taskKey"), call.task],
    ["id", t("aicalls.fact.callId"), call.id],
    ["logical", t("aicalls.fact.logicalId"), detail?.logical_call_id],
    ["correlation", t("aicalls.fact.correlationId"), detail?.correlation_id],
    ["config", t("aicalls.fact.configHash"), detail?.config_hash],
    ["sentinel", t("aicalls.fact.errorCode"), call.error_sentinel],
  ];
  return ids.flatMap(([key, term, value]) =>
    value ? [{ key, term, value: <code>{value}</code> }] : [],
  );
}

function AttemptLine({ attempt }: Readonly<{ attempt: AiCallAttempt }>) {
  const t = useT();
  const { locale } = useLocale();
  // Which rung, which binding, why it ran, how long. A reason names what went
  // wrong before it, so beside the tier it says where the walk went next.
  const answered =
    attempt.decision_choice !== undefined &&
    attempt.decision_confidence !== undefined;
  const parts = [
    attempt.tier ? tierLabel(attempt.tier, t) : "",
    attempt.model_id ? attemptBinding(attempt, t) : "",
    attempt.served_provider
      ? t("aicalls.detail.servedBy", { host: attempt.served_provider })
      : "",
    attempt.attempt_reason ? attemptReasonLabel(attempt.attempt_reason, t) : "",
    t("aicalls.ms", { value: formatNumber(attempt.latency_ms, locale) }),
    answered
      ? t("aicalls.decisionAnswer", {
          choice: attempt.decision_choice ?? "",
          confidence: formatDecimal(
            attempt.decision_confidence ?? 0,
            locale,
            2,
          ),
        })
      : "",
  ].filter(Boolean);
  return (
    <li className="aicalls-attempt">
      <span className="t-num">#{ordinalNumber(attempt.attempt)}</span>
      {attempt.kind === "decision" && (
        <Badge>{t("aicalls.badge.decision")}</Badge>
      )}
      <span className="aicalls-attempt-text">{parts.join(" · ")}</span>
      {attempt.error_sentinel && (
        <Badge tone={callCodeTone(attempt.error_sentinel)}>
          {callCodeName(attempt.error_sentinel, t)}
        </Badge>
      )}
    </li>
  );
}

function PayloadBlock({
  label,
  text,
  current,
  onCopied,
}: Readonly<{
  label: string;
  text: string;
  current: boolean;
  onCopied: () => void;
}>) {
  const t = useT();
  const copy = useClipboardCopy(
    text,
    {
      copy: t("aicalls.copy"),
      copied: t("aicalls.copied"),
      remedy: t("aicalls.copyRemedy"),
    },
    onCopied,
  );
  // The clipboard is one slot: only the block copied last may say Copied.
  const shown = copy.copied && current ? copy.label : t("aicalls.copy");
  return (
    <div className="aicalls-payload">
      <div className="aicalls-payload-head">
        <span className="t-eyebrow">{label}</span>
        <Button
          variant="ghost"
          aria-label={
            copy.copied && current
              ? copy.label
              : t("aicalls.copyNamed", { what: label })
          }
          onClick={copy.copy}
        >
          <Copy aria-hidden />
          {shown}
        </Button>
      </div>
      {copy.notice}
      <pre className="code-block aicalls-code">{text}</pre>
    </div>
  );
}

function Payload({
  detail,
  captureEnabled,
  onExport,
}: Readonly<{
  detail: AiCallRecord;
  captureEnabled: boolean;
  onExport: (detail: AiCallRecord) => void;
}>) {
  const t = useT();
  const [copiedLast, setCopiedLast] = useState<string | null>(null);
  if (!captureEnabled) {
    return <p className="t-caption">{t("aicalls.payload.off")}</p>;
  }
  if (!detail.payload_captured || !detail.payload) {
    return <p className="t-caption">{t("aicalls.payload.none")}</p>;
  }
  return (
    <>
      <PayloadBlock
        label={t("aicalls.detail.request")}
        text={payloadText(detail.payload.request)}
        current={copiedLast === "request"}
        onCopied={() => setCopiedLast("request")}
      />
      <PayloadBlock
        label={t("aicalls.detail.response")}
        text={payloadText(detail.payload.response)}
        current={copiedLast === "response"}
        onCopied={() => setCopiedLast("response")}
      />
      <div>
        <Button onClick={() => onExport(detail)}>{t("aiexport.button")}</Button>
      </div>
    </>
  );
}

/**
 * One call, opened beside the trace. A drawer rather than a row that opens in
 * place, so the table keeps its height and column widths whichever call is read.
 * The neighbouring calls are one press away.
 */
export function CallDrawer({
  call,
  captureEnabled,
  onClose,
  onPrev,
  onNext,
  onExport,
}: Readonly<{
  call: AiCallSummary;
  captureEnabled: boolean;
  onClose: () => void;
  onPrev?: () => void;
  onNext?: () => void;
  onExport: (detail: AiCallRecord) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const titleId = useId();
  const query = useCallDetail(call.id);
  const outcome = call.outcome;
  const n = (value: number) => formatNumber(value, locale);
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer-reading"
      // The reader opened a call to read it: focus lands on the drawer itself.
      // The first thing drawn as focused is then not a stepping arrow.
      initialFocusTo={() =>
        document
          .getElementById(titleId)
          ?.closest<HTMLElement>("[role=dialog]") ?? null
      }
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {aiTaskLabel(call.task, t, call.task_display_name)}
        </Heading>
        <div className="aicalls-drawer-meta">
          <StatusBadge
            kind={OUTCOME_KIND[outcome]}
            label={outcomeLabel(outcome, t)}
          />
          {call.decision_attempted && (
            <Badge>{t("aicalls.badge.decision")}</Badge>
          )}
          {call.cache_hit && <Badge>{t("aicalls.badge.cacheHit")}</Badge>}
          {call.degraded && (
            <Badge tone="warning">{t("aicalls.badge.degraded")}</Badge>
          )}
          <time className="t-caption" dateTime={call.occurred_at}>
            {formatDateTime(call.occurred_at, locale, viewerZone())}
          </time>
          <span className="aicalls-drawer-nav">
            <IconAction
              variant="ghost"
              label={t("aicalls.prev")}
              icon={<ChevronLeft aria-hidden />}
              disabled={!onPrev}
              onClick={onPrev}
            />
            <IconAction
              variant="ghost"
              label={t("aicalls.next")}
              icon={<ChevronRight aria-hidden />}
              disabled={!onNext}
              onClick={onNext}
            />
          </span>
        </div>
      </DrawerHead>
      <DrawerBody>
        <Section title={t("aicalls.detail.call")}>
          <FactList facts={summaryFacts(call, query.data, t, n, locale)} />
        </Section>
        <QueryStates query={query} pendingLabel={t("aicalls.detail.attempts")}>
          {query.data && (
            <>
              <Section title={t("aicalls.detail.attempts")}>
                <ol className="aicalls-attempts">
                  {query.data.attempts.map((attempt) => (
                    <AttemptLine key={attempt.attempt} attempt={attempt} />
                  ))}
                </ol>
              </Section>
              {/* The request settings the attempts were configured with, fixed
                  per tier rather than per call. */}
              {query.data.config?.provider_params &&
                Object.keys(query.data.config.provider_params).length > 0 && (
                  <Section title={t("aicalls.detail.settings")}>
                    <pre className="code-block aicalls-code">
                      {payloadText(query.data.config.provider_params)}
                    </pre>
                  </Section>
                )}
              <Section title={t("aicalls.detail.payload")}>
                <Payload
                  key={query.data.id}
                  detail={query.data}
                  captureEnabled={captureEnabled}
                  onExport={onExport}
                />
              </Section>
            </>
          )}
        </QueryStates>
        <Disclosure summary={t("aicalls.detail.technical")}>
          <FactList facts={technicalFacts(call, query.data, t)} />
        </Disclosure>
      </DrawerBody>
    </Modal>
  );
}
