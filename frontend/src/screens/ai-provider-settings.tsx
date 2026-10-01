// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Select } from "../design-system/select";
import { TokenInput } from "../design-system/tokeninput";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { isOpenRouter } from "./ai-provider-links";
import { ROUTING_KEY } from "./ai-routing-query";
import { problemMessageOf, throwProblem } from "./common";
import { VERTEX_PROVIDER, VertexLocationField } from "./vertex-location";

// Where a provider is reached, and for a broker which of its hosts may serve
// the request. Set once here; every lane that binds the provider reads them.

type Routing = components["schemas"]["AiRouting"];
type ProviderSettings = components["schemas"]["AiProviderSettings"];
type Upstream = components["schemas"]["AiOpenRouterUpstream"];

// The host each provider takes, and what it does with it: the chat broker gets
// /v1 appended, while a decision endpoint is the full URL, used as written.
type HostField = Readonly<{
  help: MessageKey;
  placeholder: MessageKey;
  preset?: string;
}>;
const HOST_FIELDS: ReadonlyMap<string, HostField> = new Map([
  [
    "openai_compatible",
    {
      help: "aiRouting.baseUrl.help",
      placeholder: "aiRouting.baseUrl.placeholder",
      preset: "https://openrouter.ai/api",
    },
  ],
  [
    "jev",
    {
      help: "aiRouting.baseUrl.help.jev",
      placeholder: "aiRouting.baseUrl.placeholder.jev",
    },
  ],
  [
    "jev_compatible",
    {
      help: "aiRouting.baseUrl.help.jevCompatible",
      placeholder: "aiRouting.baseUrl.placeholder.jevCompatible",
      preset: "https://openrouter.ai/api/alpha/decisions",
    },
  ],
]);

/** Whether a provider has anything this sheet can set. */
export function hasProviderSettings(provider: string): boolean {
  return HOST_FIELDS.has(provider) || provider === VERTEX_PROVIDER;
}

export function useSetProviderSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: {
      provider: string;
      settings: ProviderSettings;
    }) => {
      const { error } = await api.PUT("/ai/provider-settings/{provider}", {
        params: { path: { provider: vars.provider } },
        body: vars.settings,
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ROUTING_KEY });
      // A key test and a model list ask the provider at its host.
      queryClient.invalidateQueries({ queryKey: ["ai-available-models"] });
    },
  });
}

// What the sheet sends: only what is set. An upstream with no pins is no
// upstream at all — the provider then pins nothing — so it is left off.
function settingsBody(draft: ProviderSettings): ProviderSettings {
  const body: ProviderSettings = {};
  if (draft.base_url?.trim()) body.base_url = draft.base_url.trim();
  if (draft.location) body.location = draft.location;
  const upstream = draft.upstream;
  if (upstream?.only?.length) body.upstream = { only: upstream.only };
  if (upstream?.ignore?.length) {
    body.upstream = { ...body.upstream, ignore: upstream.ignore };
  }
  if (upstream?.allow_fallbacks !== undefined) {
    body.upstream = {
      ...body.upstream,
      allow_fallbacks: upstream.allow_fallbacks,
    };
  }
  return body;
}

export function ProviderSettingsForm({
  provider,
  routing,
  canManage,
}: Readonly<{
  provider: string;
  routing: Routing | undefined;
  canManage: boolean;
}>) {
  const t = useT();
  const stored = routing?.providers?.[provider] ?? {};
  const [draft, setDraft] = useState<ProviderSettings>(stored);
  const save = useSetProviderSettings();
  const host = HOST_FIELDS.get(provider);
  const disabled = !canManage || save.isPending;
  const broker =
    provider === "openai_compatible" && isOpenRouter(draft.base_url ?? "");
  return (
    <div className="ai-provider-settings">
      {host && (
        <div className="binding-provider-row">
          <Field label={t("aiRouting.baseUrl.label")} hint={t(host.help)}>
            {(control) => (
              <TextInput
                {...control}
                value={draft.base_url ?? ""}
                disabled={disabled}
                placeholder={t(host.placeholder)}
                onChange={(e) =>
                  setDraft({ ...draft, base_url: e.target.value })
                }
              />
            )}
          </Field>
          {host.preset && (
            <span className="binding-preset-action">
              <Button
                variant="link"
                disabled={disabled}
                onClick={() => setDraft({ ...draft, base_url: host.preset })}
              >
                {t("aiProviderSettings.preset.openrouter")}
              </Button>
            </span>
          )}
        </div>
      )}
      {provider === VERTEX_PROVIDER && (
        <VertexLocationField
          value={draft.location ?? ""}
          profile={routing?.profile ?? ""}
          disabled={disabled}
          onChange={(location) => setDraft({ ...draft, location })}
        />
      )}
      {broker && (
        <UpstreamFields
          upstream={draft.upstream ?? {}}
          disabled={disabled}
          onChange={(upstream) => setDraft({ ...draft, upstream })}
        />
      )}
      <div className="ai-provider-settings-foot">
        <Button
          variant="primary"
          pending={save.isPending}
          disabled={!canManage}
          reason={canManage ? undefined : t("aiProviderKeys.adminOnly")}
          onClick={() =>
            save.mutate({ provider, settings: settingsBody(draft) })
          }
        >
          {t("aiProviderSettings.save")}
        </Button>
      </div>
      {save.error ? (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("aiProviderKeys.saveFailed")}
        >
          {problemMessageOf(save.error, t)}
        </Callout>
      ) : null}
    </div>
  );
}

const FALLBACK_OPTIONS = ["default", "yes", "no"] as const;
type Fallback = (typeof FALLBACK_OPTIONS)[number];

function fallbackOf(upstream: Upstream): Fallback {
  if (upstream.allow_fallbacks === undefined) return "default";
  return upstream.allow_fallbacks ? "yes" : "no";
}

// Which of OpenRouter's hosts may serve this provider's requests, for every
// lane on it: a residency pin lives here. How one model is served stays on the
// tier.
function UpstreamFields({
  upstream,
  disabled,
  onChange,
}: Readonly<{
  upstream: Upstream;
  disabled: boolean;
  onChange: (next: Upstream) => void;
}>) {
  const t = useT();
  return (
    <fieldset className="ai-provider-upstream">
      <legend className="t-h3">{t("aiProviderSettings.upstream.label")}</legend>
      <p className="t-caption">{t("aiProviderSettings.upstream.help")}</p>
      <Field label={t("aiProviderSettings.upstream.only")}>
        {(control) => (
          <TokenInput
            {...control}
            values={upstream.only ?? []}
            disabled={disabled}
            onChange={(only) => onChange({ ...upstream, only: [...only] })}
          />
        )}
      </Field>
      <Field label={t("aiProviderSettings.upstream.ignore")}>
        {(control) => (
          <TokenInput
            {...control}
            values={upstream.ignore ?? []}
            disabled={disabled}
            onChange={(ignore) =>
              onChange({ ...upstream, ignore: [...ignore] })
            }
          />
        )}
      </Field>
      <Field label={t("aiProviderSettings.upstream.fallbacks")}>
        {(control) => (
          <Select
            {...control}
            value={fallbackOf(upstream)}
            disabled={disabled}
            options={FALLBACK_OPTIONS.map((value) => ({
              value,
              label: t(`aiProviderSettings.upstream.fallbacks.${value}`),
            }))}
            onChange={(value) =>
              onChange({
                ...upstream,
                allow_fallbacks:
                  value === "default" ? undefined : value === "yes",
              })
            }
          />
        )}
      </Field>
    </fieldset>
  );
}
