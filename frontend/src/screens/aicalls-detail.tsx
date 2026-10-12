import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, Card } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { KeyedName } from "../design-system/keyedname";
import { formatDecimal, formatNumber, ordinalNumber } from "../format/format";
import { type Translator, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  attemptReasonLabel,
  callCodeName,
  tierLabel,
} from "./ai-decision-labels";
import { providerName } from "./ai-provider-names";
import { ExportScenarioDialog } from "./aiexport";
import { QueryStates, unwrap } from "./common";

// A string response is shown verbatim (real newlines); an object is
// pretty-printed. Either way the .code-block surface wraps and scrolls it.
function payloadText(value: unknown): string {
  return typeof value === "string" ? value : JSON.stringify(value, null, 2);
}

// One attempt's model and its vendor's name, the pair the call row shows.
function attemptBinding(
  attempt: components["schemas"]["AiCallAttempt"],
  t: Translator,
): string {
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

export function CallDetailPanel({
  id,
  captureEnabled,
}: Readonly<{ id: string; captureEnabled: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const [exporting, setExporting] = useState(false);
  const query = useQuery({
    queryKey: ["ai-call", id],
    queryFn: async () => {
      return unwrap(
        await api.GET("/ai/calls/{id}", {
          params: { path: { id } },
        }),
      );
    },
  });
  return (
    <QueryStates query={query} pendingLabel={t("aicalls.title")}>
      {query.data && (
        <Card as="div" inset className="aicalls-detail">
          <p>
            {query.data.model_id
              ? t("aicalls.detail.identity", {
                  served: query.data.served_model,
                  provider: providerName(query.data.provider, t),
                  configured: query.data.model_id,
                })
              : t("aicalls.detail.identityNoModel", {
                  served: query.data.served_model,
                  provider: providerName(query.data.provider, t),
                })}
          </p>
          <p>{identitySource(query.data.served_identity_source, t)}</p>
          <p>
            {query.data.context_scopes.length > 0
              ? t("aicalls.detail.context", {
                  scopes: query.data.context_scopes.join(", "),
                })
              : t("aicalls.detail.contextNone")}
          </p>
          <Heading size="xsmall" as="h4">
            {t("aicalls.detail.attempts")}
          </Heading>
          <ol>
            {query.data.attempts.map((attempt) => (
              <li key={attempt.attempt}>
                <span className="t-num">#{ordinalNumber(attempt.attempt)}</span>{" "}
                {attempt.kind === "decision" && (
                  <Badge>{t("aicalls.badge.decision")}</Badge>
                )}{" "}
                {/* Which rung this attempt ran on, then why it ran. A reason
                    names what went wrong before it, so read beside the tier it
                    says where the walk went next. */}
                {attempt.tier ? `${tierLabel(attempt.tier, t)} · ` : ""}
                {/* The binding this attempt asked. The call's own is the
                    terminal rung's, so a decision attempt names a model the row does not. */}
                {attempt.model_id ? `${attemptBinding(attempt, t)} · ` : ""}
                {attempt.served_provider
                  ? `${t("aicalls.detail.servedBy", { host: attempt.served_provider })} · `
                  : ""}
                {/* An ordinary first attempt and a decision that stood ran
                    for no reason worth naming, so they print none rather than
                    a placeholder between the binding and the latency. */}
                {attempt.attempt_reason
                  ? `${attemptReasonLabel(attempt.attempt_reason, t)} · `
                  : ""}
                {t("aicalls.ms", {
                  value: formatNumber(attempt.latency_ms, locale),
                })}
                {/* What the decision model answered, whether or not it stood:
                    the reason on the NEXT rung says why the walk went on. The
                    label is a wire value the site owns, shown as sent. */}
                {attempt.decision_choice !== undefined &&
                  attempt.decision_confidence !== undefined &&
                  ` · ${t("aicalls.decisionAnswer", {
                    choice: attempt.decision_choice,
                    confidence: formatDecimal(
                      attempt.decision_confidence,
                      locale,
                      2,
                    ),
                  })}`}
                {attempt.error_sentinel && (
                  <KeyedName
                    name={callCodeName(attempt.error_sentinel, t)}
                    code={attempt.error_sentinel}
                  />
                )}
              </li>
            ))}
          </ol>
          {/* The request settings the attempts were configured with: the
              broker's provider and reasoning blocks, the task's level and the
              deadline, fixed per tier rather than per call. */}
          {query.data.config?.provider_params &&
          Object.keys(query.data.config.provider_params).length > 0 ? (
            <div className="field">
              <span className="code-label t-eyebrow">
                {t("aicalls.detail.settings")}
              </span>
              <pre className="code-block">
                {payloadText(query.data.config.provider_params)}
              </pre>
            </div>
          ) : null}
          {!captureEnabled ? (
            <p>{t("aicalls.payload.off")}</p>
          ) : !query.data.payload_captured || !query.data.payload ? (
            <p>{t("aicalls.payload.none")}</p>
          ) : (
            <>
              <div className="form-stack">
                <div className="field">
                  <span className="code-label t-eyebrow">
                    {t("aicalls.detail.request")}
                  </span>
                  <pre className="code-block">
                    {payloadText(query.data.payload.request)}
                  </pre>
                </div>
                <div className="field">
                  <span className="code-label t-eyebrow">
                    {t("aicalls.detail.response")}
                  </span>
                  <pre className="code-block">
                    {payloadText(query.data.payload.response)}
                  </pre>
                </div>
                <div>
                  <Button onClick={() => setExporting(true)}>
                    {t("aiexport.button")}
                  </Button>
                </div>
              </div>
              {exporting && (
                <ExportScenarioDialog
                  call={query.data}
                  onClose={() => setExporting(false)}
                />
              )}
            </>
          )}
        </Card>
      )}
    </QueryStates>
  );
}
