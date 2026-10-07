// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Checkbox, Field, TextInput } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import "./ai-settings.css";

// Which OpenRouter hosts may read this connection's requests, and under what
// privacy. Set on the connection only: every tier on it is served under the
// same rules, and the server refuses a tier that writes one of these keys, so
// no tier can loosen what is set here.

type Upstream = components["schemas"]["AiOpenRouterUpstream"];

export const OPENROUTER_PRIVACY_DOCS =
  "https://openrouter.ai/docs/guides/features/zdr";
export const OPENROUTER_ROUTING_DOCS =
  "https://openrouter.ai/docs/guides/routing/provider-selection";

export type OpenRouterDraft = Readonly<{
  zdr: boolean;
  deny: boolean;
  distill: boolean;
  fallbacks: boolean;
  only: string;
  ignore: string;
}>;

/** The stored keys as the form's draft. */
export function draftOf(upstream: Upstream | undefined): OpenRouterDraft {
  return {
    zdr: upstream?.zdr === true,
    deny: upstream?.data_collection === "deny",
    distill: upstream?.enforce_distillable_text === true,
    fallbacks: upstream?.allow_fallbacks !== false,
    only: (upstream?.only ?? []).join(", "),
    ignore: (upstream?.ignore ?? []).join(", "),
  };
}

function hosts(list: string): string[] {
  return [
    ...new Set(
      list
        .split(",")
        .map((h) => h.trim())
        .filter(Boolean),
    ),
  ];
}

/**
 * The draft as the connection's keys. A rule left at OpenRouter's own default
 * is not written, so the request carries only what an admin chose; undefined
 * when nothing is.
 */
export function upstreamOf(draft: OpenRouterDraft): Upstream | undefined {
  const out: Upstream = {};
  if (draft.zdr) out.zdr = true;
  if (draft.deny) out.data_collection = "deny";
  if (draft.distill) out.enforce_distillable_text = true;
  if (!draft.fallbacks) out.allow_fallbacks = false;
  const only = hosts(draft.only);
  const ignore = hosts(draft.ignore);
  if (only.length) out.only = only;
  if (ignore.length) out.ignore = ignore;
  return Object.keys(out).length ? out : undefined;
}

export function OpenRouterSettings({
  draft,
  onChange,
  disabled,
}: Readonly<{
  draft: OpenRouterDraft;
  onChange: (next: OpenRouterDraft) => void;
  disabled: boolean;
}>) {
  const t = useT();
  const toggle = (
    key: "zdr" | "deny" | "distill" | "fallbacks",
    label: string,
    help: string,
  ) => (
    <Checkbox
      className="ai-openrouter-toggle"
      checked={draft[key]}
      disabled={disabled}
      onChange={(e) => onChange({ ...draft, [key]: e.target.checked })}
      label={
        <span>
          <span>{label}</span>
          <span className="t-caption ai-openrouter-help">{help}</span>
        </span>
      }
    />
  );
  return (
    <section
      className="ai-openrouter-settings"
      aria-label={t("aiOpenRouter.title")}
    >
      <div className="ai-figures-head">
        <Heading size="small" className="t-h3">
          {t("aiOpenRouter.title")}
        </Heading>
        <span className="t-caption">
          <a href={OPENROUTER_PRIVACY_DOCS} target="_blank" rel="noreferrer">
            {t("aiOpenRouter.privacyDocs")}
          </a>
          {" · "}
          <a href={OPENROUTER_ROUTING_DOCS} target="_blank" rel="noreferrer">
            {t("aiOpenRouter.routingDocs")}
          </a>
        </span>
      </div>
      <p className="t-caption">{t("aiOpenRouter.intro")}</p>
      <div className="ai-openrouter-toggles">
        {toggle("zdr", t("aiOpenRouter.zdr"), t("aiOpenRouter.zdr.help"))}
        {toggle("deny", t("aiOpenRouter.deny"), t("aiOpenRouter.deny.help"))}
        {toggle(
          "distill",
          t("aiOpenRouter.distill"),
          t("aiOpenRouter.distill.help"),
        )}
        {toggle(
          "fallbacks",
          t("aiOpenRouter.fallbacks"),
          t("aiOpenRouter.fallbacks.help"),
        )}
      </div>
      <Field label={t("aiOpenRouter.only")} hint={t("aiOpenRouter.only.help")}>
        {(control) => (
          <TextInput
            {...control}
            value={draft.only}
            disabled={disabled}
            placeholder={t("aiOpenRouter.only.placeholder")}
            onChange={(e) => onChange({ ...draft, only: e.target.value })}
          />
        )}
      </Field>
      <Field
        label={t("aiOpenRouter.ignore")}
        hint={t("aiOpenRouter.ignore.help")}
      >
        {(control) => (
          <TextInput
            {...control}
            value={draft.ignore}
            disabled={disabled}
            placeholder={t("aiOpenRouter.ignore.placeholder")}
            onChange={(e) => onChange({ ...draft, ignore: e.target.value })}
          />
        )}
      </Field>
      <p className="t-caption">{t("aiOpenRouter.account")}</p>
    </section>
  );
}
