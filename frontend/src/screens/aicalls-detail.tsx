import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, Card } from "../design-system/atoms";
import { Eyebrow } from "../design-system/eyebrow";
import { formatDecimal, formatNumber, ordinalNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { attemptReasonLabel, tierLabel } from "./ai-decision-labels";
import { ExportScenarioDialog } from "./aiexport";
import { QueryStates, throwProblem } from "./common";

// A string response is shown verbatim (real newlines); an object is
// pretty-printed. Either way the .code-block surface wraps and scrolls it.
function payloadText(value: unknown): string {
  return typeof value === "string" ? value : JSON.stringify(value, null, 2);
}

// One attempt's binding as `provider/model`, the spelling the call row uses. A
// row with no provider still names its model rather than a stray slash.
function attemptBinding(
  attempt: components["schemas"]["AiCallAttempt"],
): string {
  return attempt.provider
    ? `${attempt.provider}/${attempt.model_id}`
    : (attempt.model_id ?? "");
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
      const { data, error } = await api.GET("/ai/calls/{id}", {
        params: { path: { id } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  return (
    <QueryStates query={query} pendingLabel={t("aicalls.title")}>
      {query.data && (
        <Card as="div" inset className="aicalls-detail">
          <p>
            {t("aicalls.detail.identity", {
              served: query.data.served_model,
              provider: query.data.provider,
              configured: query.data.model_id,
            })}
          </p>
          <p>
            {t("aicalls.detail.source", {
              source: query.data.served_identity_source,
            })}
          </p>
          <p>
            {query.data.context_scopes.length > 0
              ? t("aicalls.detail.context", {
                  scopes: query.data.context_scopes.join(", "),
                })
              : t("aicalls.detail.contextNone")}
          </p>
          {/* A bare h3 carries no class, and preflight leaves it at body size
              and body weight — a heading only the document tree can see. The
              eyebrow is the one spelling of a label over a block, and `as="h3"`
              is what keeps it a real heading inside the card's own h2. */}
          <Eyebrow as="h3">{t("aicalls.detail.attempts")}</Eyebrow>
          <ol>
            {query.data.attempts.map((attempt) => (
              <li key={attempt.attempt}>
                <span className="t-num">#{ordinalNumber(attempt.attempt)}</span>{" "}
                {attempt.kind === "decision" && (
                  <Badge>{t("aicalls.badge.decision")}</Badge>
                )}{" "}
                {/* Which rung this attempt ran on, then why it ran. A reason
                    names what went wrong BEFORE it, so read beside the tier it
                    says where the walk went next. */}
                {attempt.tier ? `${tierLabel(attempt.tier, t)} · ` : ""}
                {/* The binding THIS attempt asked, spelled as the row above
                    spells the call's. The call's own binding is the terminal
                    rung, so a decision attempt that fell through names a model
                    nothing else on the page does. */}
                {attempt.model_id ? `${attemptBinding(attempt)} · ` : ""}
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
                  <Badge tone="danger">{attempt.error_sentinel}</Badge>
                )}
              </li>
            ))}
          </ol>
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
