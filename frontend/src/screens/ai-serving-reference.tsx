// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Disclosure } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import { type Source, summarize } from "./ai-request-summary";
import { throwProblem } from "./common";
import "./ai-settings.css";

// What the serving editor reads beside the text: every field the served schema
// documents, and the request the server merged for the tier, in plain words.

type ServingValue = components["schemas"]["AiOpenRouterRouting"];

function useRoutingSchema() {
  return useQuery({
    queryKey: ["ai-routing-schema"],
    staleTime: Infinity,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/routing/schema");
      if (error) throwProblem(error);
      return data;
    },
  });
}

type FieldDoc = Readonly<{
  description?: string;
  "x-doc-url"?: string;
  "x-placement"?: string;
  enum?: string[];
  type?: string;
  oneOf?: unknown[];
}>;

function isDocs(value: unknown): value is Record<string, FieldDoc> {
  return typeof value === "object" && value !== null;
}

function fieldsOf(schema: unknown, def: string): [string, FieldDoc][] {
  if (!isDocs(schema)) return [];
  const block = schema[def];
  const properties =
    isDocs(block) && "properties" in block ? block.properties : undefined;
  return isDocs(properties) ? Object.entries(properties) : [];
}

function typeOf(field: FieldDoc): string {
  if (field.enum) return field.enum.join(" | ");
  if (field.oneOf) return "value or object";
  return field.type ?? "value";
}

const EMBEDDINGS_FIELDS = [
  "only",
  "ignore",
  "allow_fallbacks",
  "zdr",
  "data_collection",
  "enforce_distillable_text",
];

/** Every field the served schema documents, with a chip on the connection's. */
export function FieldReference({
  embeddings,
}: Readonly<{ embeddings: boolean }>) {
  const t = useT();
  const schema = useRoutingSchema();
  const provider = fieldsOf(schema.data, "openRouterProvider").filter(
    ([key]) => !embeddings || EMBEDDINGS_FIELDS.includes(key),
  );
  const reasoning = embeddings
    ? []
    : fieldsOf(schema.data, "openRouterReasoning");
  return (
    <aside className="ai-serving-reference">
      <Heading size="xsmall">{t("aiServing.reference")}</Heading>
      <dl>
        {[
          ...provider.map(([k, f]) => [`provider.${k}`, f] as const),
          ...reasoning.map(([k, f]) => [`reasoning.${k}`, f] as const),
        ].map(([key, field]) => (
          <div key={key}>
            <dt>
              <code>{key}</code>{" "}
              <span className="t-caption">{typeOf(field)}</span>
              {!embeddings && field["x-placement"] === "connection" ? (
                <Badge tone="warning">{t("aiServing.connectionOnly")}</Badge>
              ) : null}
            </dt>
            <dd className="t-caption">
              {field.description}{" "}
              {field["x-doc-url"] ? (
                <a href={field["x-doc-url"]} target="_blank" rel="noreferrer">
                  {t("aiServing.openRouterDocs")}
                </a>
              ) : null}
            </dd>
          </div>
        ))}
      </dl>
      <p className="t-caption">{t("aiServing.referenceNote")}</p>
    </aside>
  );
}

const SOURCE_LABEL: Record<
  Source,
  | "aiServing.source.default"
  | "aiServing.source.connection"
  | "aiServing.source.tier"
  | "aiServing.source.task"
> = {
  default: "aiServing.source.default",
  connection: "aiServing.source.connection",
  tier: "aiServing.source.tier",
  task: "aiServing.source.task",
};

export function RequestSummary({
  lane,
  effective,
  written,
  pending,
}: Readonly<{
  lane: string;
  effective: ServingValue | undefined;
  written: ServingValue | undefined;
  pending: boolean;
}>) {
  const t = useT();
  const lines = effective ? summarize(effective, written, true) : [];
  return (
    <section className="ai-serving-summary">
      <div className="ai-figures-head">
        <Heading size="xsmall">{t("aiServing.asked")}</Heading>
        <span className="t-caption">{t("aiServing.askedFor", { lane })}</span>
      </div>
      {!effective ? (
        <p className="t-caption">
          {pending ? t("aiServing.checking") : t("aiServing.fixFirst")}
        </p>
      ) : lines.length === 0 ? (
        <p className="t-caption">{t("aiServing.brokerOwn")}</p>
      ) : (
        <ul className="ai-serving-lines">
          {lines.map((line) => (
            <li key={line.key}>
              <Badge
                tone={
                  line.source === "tier"
                    ? "accent"
                    : line.source === "connection"
                      ? "warning"
                      : undefined
                }
              >
                {t(SOURCE_LABEL[line.source])}
              </Badge>
              <span>
                <span>{t(line.sentence.key, line.sentence.params)}</span>
                {line.value ? (
                  <code className="t-caption">
                    {line.key} = {line.value}
                  </code>
                ) : null}
              </span>
            </li>
          ))}
        </ul>
      )}
      <Disclosure summary={t("aiServing.showJson")}>
        <pre className="code-block">
          {JSON.stringify(effective ?? {}, null, 2)}
        </pre>
      </Disclosure>
    </section>
  );
}
