// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { Check } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import {
  JsonField,
  type JsonProblem,
  lineOfPath,
  parseProblem,
} from "../design-system/jsonfield";
import { formatNumber, identifierNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { OPENROUTER_ROUTING_DOCS } from "./ai-openrouter-settings";
import { isOpenRouter } from "./ai-provider-links";
import { type SliceValue, withSlice } from "./ai-routing-slice";
import { FieldReference, RequestSummary } from "./ai-serving-reference";
import { problemMessageOf, throwProblem } from "./common";
import "./ai-settings.css";

// How OpenRouter serves one lane, as the JSON OpenRouter itself takes. The
// browser checks only that the text parses; the server's preview names every
// bad key by its path and returns the request the lane will send, so the
// merge shown here is the server's and never a copy of it.

type Routing = components["schemas"]["AiRouting"];
type ServingValue = components["schemas"]["AiOpenRouterRouting"];
type Preview = components["schemas"]["AiRoutingPreview"];

/** Why the Serving section shows no editor, or null when it does. */
export type ServingBlocked = "decisions" | "provider" | "host" | null;

export function servingBlocked(
  value: SliceValue,
  routing: Routing,
): ServingBlocked {
  if (value.kind === "decisions") return "decisions";
  if (value.binding.provider !== "openai_compatible") return "provider";
  const host =
    value.binding.base_url ||
    routing.providers?.openai_compatible?.base_url ||
    "";
  return isOpenRouter(host) ? null : "host";
}

const PREVIEW_DELAY_MS = 400;

const SAMPLE_TIER = JSON.stringify(
  {
    provider: {
      sort: { by: "throughput", partition: "model" },
      quantizations: ["fp16", "bf16"],
      preferred_max_latency: { p90: 4 },
      require_parameters: true,
    },
    reasoning: { exclude: true },
  },
  null,
  2,
);

const SAMPLE_EMBEDDINGS = JSON.stringify(
  { provider: { only: ["mistral"], allow_fallbacks: false } },
  null,
  2,
);

function isServingValue(value: unknown): value is ServingValue {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function textOf(routing: ServingValue | undefined): string {
  return routing ? JSON.stringify(routing, null, 2) : "";
}

/** Where a lane's routing sits in the routing document. */
function routingPath(value: SliceValue): string {
  return value.kind === "tier"
    ? `tiers.${value.tier}.routing`
    : "embeddings.routing";
}

type Unreadable = Readonly<{
  line?: number;
  why: "aiServing.notJson" | "aiServing.notObject";
}>;

/** The value the text says, or why it says nothing the server can read. */
function readText(text: string): {
  routing: ServingValue | undefined;
  problem: Unreadable | null;
} {
  const unparsed = parseProblem(text);
  if (unparsed)
    return {
      routing: undefined,
      problem: { line: unparsed.line, why: "aiServing.notJson" },
    };
  if (text.trim() === "") return { routing: undefined, problem: null };
  const parsed: unknown = JSON.parse(text);
  return isServingValue(parsed)
    ? { routing: parsed, problem: null }
    : { routing: undefined, problem: { line: 1, why: "aiServing.notObject" } };
}

// Debounced on the document's TEXT: a draft rebuilt each render is a new
// object every time, and an object key would restart the wait forever.
function usePreview(body: Routing | null, delayMs: number, enabled: boolean) {
  const key = body === null ? null : JSON.stringify(body);
  const [debounced, setDebounced] = useState({ key, body });
  // biome-ignore lint/correctness/useExhaustiveDependencies: keyed on the body's text; the object is new every render
  useEffect(() => {
    const timer = setTimeout(() => setDebounced({ key, body }), delayMs);
    return () => clearTimeout(timer);
  }, [key, delayMs]);
  const query = useQuery({
    queryKey: ["ai-routing-preview", debounced.key],
    // A reader who may not save is not sent a check: the preview is a write's
    // rehearsal and asks the write's grants.
    enabled: enabled && debounced.body !== null,
    queryFn: async () => {
      if (debounced.body === null) return null;
      const { data, error } = await api.POST("/ai/routing/preview", {
        body: debounced.body,
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  return { query, checking: debounced.key !== key || query.isFetching };
}

/**
 * The Serving section of the binding dialog. `onChange` hands back the lane's
 * routing (undefined for the shipped default); `onValid` says whether a save
 * may go: the text parses and the server's preview names no problem.
 */
export function ServingSection({
  value,
  routing,
  disabled,
  onChange,
  onValid,
  previewDelayMs = PREVIEW_DELAY_MS,
}: Readonly<{
  value: SliceValue;
  routing: Routing;
  disabled: boolean;
  onChange: (routing: ServingValue | undefined) => void;
  onValid: (valid: boolean) => void;
  // How long typing pauses before the server is asked; a test sets 0.
  previewDelayMs?: number;
}>) {
  const t = useT();
  const blocked = servingBlocked(value, routing);
  return (
    <section className="ai-serving">
      <div className="ai-figures-head">
        <Heading size="small" className="t-h3">
          {t("aiServing.title")}{" "}
          {blocked === null && <Badge>{t("aiServing.openRouter")}</Badge>}
        </Heading>
        <a
          className="t-caption"
          href={OPENROUTER_ROUTING_DOCS}
          target="_blank"
          rel="noreferrer"
        >
          {t("aiServing.guide")}
        </a>
      </div>
      {blocked === null && value.kind !== "decisions" ? (
        <ServingEditor
          value={value}
          routing={routing}
          disabled={disabled}
          onChange={onChange}
          onValid={onValid}
          previewDelayMs={previewDelayMs}
        />
      ) : (
        <p className="t-caption ai-serving-note">
          {t(BLOCKED_NOTE[blocked ?? "provider"], {
            provider: value.binding?.provider ?? "",
          })}
        </p>
      )}
    </section>
  );
}

const BLOCKED_NOTE = {
  decisions: "aiServing.blocked.decisions",
  provider: "aiServing.blocked.provider",
  host: "aiServing.blocked.host",
} as const;

function ServingEditor({
  value,
  routing,
  disabled,
  onChange,
  onValid,
  previewDelayMs,
}: Readonly<{
  value: Exclude<SliceValue, { kind: "decisions" }>;
  routing: Routing;
  disabled: boolean;
  onChange: (routing: ServingValue | undefined) => void;
  onValid: (valid: boolean) => void;
  previewDelayMs: number;
}>) {
  const t = useT();
  const problemsId = useId();
  const [text, setText] = useState(() => textOf(value.binding.routing));
  const read = readText(text);
  const path = routingPath(value);
  const body = read.problem
    ? null
    : withSlice(routing, withRouting(value, read.routing));
  const { query, checking } = usePreview(body, previewDelayMs, !disabled);
  const problems = problemsOf(read.problem, query, path, text, t);
  const valid =
    !read.problem && !checking && query.isSuccess && problems.length === 0;
  useEffect(() => onValid(valid), [valid, onValid]);
  const edit = (next: string) => {
    setText(next);
    const parsed = readText(next);
    if (!parsed.problem) onChange(parsed.routing);
  };
  const tierKey = value.kind === "tier" ? value.tier : "";
  const effective = tierKey ? query.data?.effective?.tiers[tierKey] : undefined;
  const sample = value.kind === "embeddings" ? SAMPLE_EMBEDDINGS : SAMPLE_TIER;
  const status = disabled ? null : statusOf(checking, problems.length, text);
  return (
    <div className="ai-serving-grid">
      <div className="form-stack">
        <p className="t-caption">{t("aiServing.empty")}</p>
        <JsonField
          aria-label={t("aiServing.json")}
          value={text}
          placeholder={sample}
          disabled={disabled}
          onChange={edit}
          problemLines={problems.flatMap((p) => (p.line ? [p.line] : []))}
          invalid={problems.length > 0}
          aria-describedby={problems.length > 0 ? problemsId : undefined}
        />
        <div className="ai-serving-actions">
          <Button
            disabled={disabled || !!read.problem || text.trim() === ""}
            onClick={() => edit(JSON.stringify(JSON.parse(text), null, 2))}
          >
            {t("aiServing.format")}
          </Button>
          <Button disabled={disabled} onClick={() => edit(sample)}>
            {t("aiServing.example")}
          </Button>
          <Button disabled={disabled} onClick={() => edit("")}>
            {t("aiServing.useDefault")}
          </Button>
          <span className="t-caption" aria-live="polite">
            {status === "aiServing.valid" ? (
              <Check aria-hidden size={14} />
            ) : null}
            {statusText(t, status)}
          </span>
        </div>
        <ProblemList id={problemsId} problems={problems} />
      </div>
      <FieldReference embeddings={value.kind === "embeddings"} />
      {value.kind === "tier" && (
        <RequestSummary
          lane={value.tier}
          effective={problems.length || read.problem ? undefined : effective}
          written={read.routing}
          pending={checking}
        />
      )}
    </div>
  );
}

/** What the line beside the buttons says. */
function statusOf(
  checking: boolean,
  problems: number,
  text: string,
): MessageKey | null {
  if (checking) return "aiServing.checking";
  if (problems) return null;
  return text.trim() ? "aiServing.valid" : "aiServing.shippedDefault";
}

/**
 * Every problem the editor shows: the text's own, or the server's for this
 * lane, or the check itself failing — which must never read as valid.
 */
function problemsOf(
  unreadable: Unreadable | null,
  query: { isError: boolean; error: unknown; data: Preview | null | undefined },
  path: string,
  text: string,
  t: ReturnType<typeof useT>,
): JsonProblem[] {
  if (unreadable)
    return [{ line: unreadable.line, message: t(unreadable.why) }];
  if (query.isError)
    return [
      {
        message: t("aiServing.previewFailed", {
          reason: problemMessageOf(query.error, t),
        }),
      },
    ];
  return serverProblems(query.data, path, text);
}

function ProblemList({
  id,
  problems,
}: Readonly<{ id: string; problems: JsonProblem[] }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  if (problems.length === 0) return null;
  return (
    <div className="ai-serving-problems" id={id}>
      <strong>
        {plural("aiServing.problems", problems.length, {
          count: formatNumber(problems.length, locale),
        })}
      </strong>
      {problems.map((p) => (
        <ErrorLine key={`${p.path}-${p.message}`}>
          {p.line ? <code>{`L${identifierNumber(p.line)}`}</code> : null}{" "}
          {p.path ? <code>{p.path}</code> : null} {p.message}
        </ErrorLine>
      ))}
    </div>
  );
}

function statusText(
  t: ReturnType<typeof useT>,
  key: MessageKey | null,
): string {
  return key ? t(key) : "";
}

/** The lane with its routing replaced, of the same kind. */
function withRouting(
  value: Exclude<SliceValue, { kind: "decisions" }>,
  next: ServingValue | undefined,
): SliceValue {
  return value.kind === "tier"
    ? { ...value, binding: { ...value.binding, routing: next } }
    : { ...value, binding: { ...value.binding, routing: next } };
}

/** The preview's problems on this lane, each placed on its line. */
function serverProblems(
  preview: Preview | null | undefined,
  path: string,
  text: string,
): JsonProblem[] {
  return (preview?.errors ?? [])
    .filter((e) => e.field === path || e.field.startsWith(`${path}.`))
    .map((e) => {
      const local = e.field.slice(path.length + 1);
      return {
        path: local || undefined,
        line: local ? lineOfPath(text, local) : undefined,
        message: e.message,
      };
    });
}
