// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  draftOf,
  type OpenRouterDraft,
  OpenRouterSettings,
  upstreamOf,
} from "./ai-openrouter-settings";
import { invalidateProviderHealth } from "./ai-provider-health";
import { isOpenRouter } from "./ai-provider-links";
import { ROUTING_KEY } from "./ai-routing-query";
import { problemMessageOf, throwProblem } from "./common";
import { VERTEX_PROVIDER, VertexLocationField } from "./vertex-location";

// Where a provider is reached. Set once here; every lane that binds the
// provider reads it. A known service fills its own host, so the reader picks a
// name rather than remembering a URL; Other asks for the URL with an example.

type Routing = components["schemas"]["AiRouting"];
type ProviderSettings = components["schemas"]["AiProviderSettings"];

// The guide a reader setting up a service by hand is pointed at.
const HOST_GUIDE =
  "https://github.com/margince/margince/blob/main/docs/how-to/connect-a-cloud-model-provider.md";

type Service = Readonly<{
  id: string;
  label: MessageKey;
  // The host it saves; empty means the adapter's compiled default.
  host: string;
  note?: MessageKey;
  noteLink?: string;
}>;

// What the Other service asks for on each provider: a chat host gets /v1
// appended, Gemini's carries its version, a decision endpoint is used as written.
type OtherHost = Readonly<{ help: MessageKey; placeholder: MessageKey }>;

type ProviderServices = Readonly<{
  services: readonly Service[];
  other: OtherHost & { label: MessageKey };
}>;

const OTHER = "other";

// Langdock serves each vendor's wire under its own path, once per region, on
// one key; `version` is the segment the adapter expects in its host.
function langdock(
  wire: string,
  version = "",
  note?: MessageKey,
): readonly Service[] {
  return [
    {
      id: "langdock-eu",
      label: "aiProviderSettings.service.langdockEu",
      host: `https://api.langdock.com/${wire}/eu${version}`,
      note,
    },
    {
      id: "langdock-us",
      label: "aiProviderSettings.service.langdockUs",
      host: `https://api.langdock.com/${wire}/us${version}`,
      note,
    },
  ];
}

const SERVICES: ReadonlyMap<string, ProviderServices> = new Map([
  [
    "openai_compatible",
    {
      services: [
        {
          id: "openrouter",
          label: "aiProviderSettings.service.openrouter",
          host: "https://openrouter.ai/api",
        },
        {
          id: "openrouter-eu",
          label: "aiProviderSettings.service.openrouterEu",
          host: "https://eu.openrouter.ai/api",
          note: "aiProviderSettings.service.openrouterEu.note",
          noteLink: "https://openrouter.ai/docs/guides/features/sovereign-ai",
        },
        {
          id: "mistral",
          label: "aiProviderSettings.service.mistral",
          host: "https://api.mistral.ai",
        },
        {
          id: "together",
          label: "aiProviderSettings.service.together",
          host: "https://api.together.xyz",
        },
        {
          id: "groq",
          label: "aiProviderSettings.service.groq",
          host: "https://api.groq.com/openai",
        },
        {
          id: "deepseek",
          label: "aiProviderSettings.service.deepseek",
          host: "https://api.deepseek.com",
        },
        ...langdock("openai"),
      ],
      other: {
        label: "aiProviderSettings.service.otherChat",
        help: "aiRouting.baseUrl.help",
        placeholder: "aiRouting.baseUrl.placeholder",
      },
    },
  ],
  [
    "gemini",
    {
      services: [
        {
          id: "google-ai-studio",
          label: "aiProviderSettings.service.googleAiStudio",
          host: "",
        },
        // Its Gemini path serves no embedder, and the embeddings lane on
        // gemini follows this host.
        ...langdock(
          "google",
          "/v1beta",
          "aiProviderSettings.service.langdockGemini.note",
        ),
      ],
      other: {
        label: "aiProviderSettings.service.otherGemini",
        help: "aiRouting.baseUrl.help.gemini",
        placeholder: "aiRouting.baseUrl.placeholder.gemini",
      },
    },
  ],
  [
    "openai",
    {
      services: [
        { id: "openai", label: "aiProviderSettings.service.openai", host: "" },
        ...langdock("openai"),
      ],
      other: {
        label: "aiProviderSettings.service.otherOpenai",
        help: "aiRouting.baseUrl.help.openai",
        placeholder: "aiRouting.baseUrl.placeholder.openai",
      },
    },
  ],
  [
    "anthropic",
    {
      services: [
        {
          id: "anthropic",
          label: "aiProviderSettings.service.anthropic",
          host: "",
        },
        ...langdock("anthropic"),
      ],
      other: {
        label: "aiProviderSettings.service.otherAnthropic",
        help: "aiRouting.baseUrl.help.anthropic",
        placeholder: "aiRouting.baseUrl.placeholder.anthropic",
      },
    },
  ],
  [
    "jev_compatible",
    {
      services: [
        {
          id: "openrouter",
          label: "aiProviderSettings.service.openrouter",
          host: "https://openrouter.ai/api/alpha/decisions",
        },
        {
          id: "openrouter-eu",
          label: "aiProviderSettings.service.openrouterEu",
          host: "https://eu.openrouter.ai/api/alpha/decisions",
          note: "aiProviderSettings.service.openrouterEu.note",
          noteLink: "https://openrouter.ai/docs/guides/features/sovereign-ai",
        },
      ],
      other: {
        label: "aiProviderSettings.service.otherDecisions",
        help: "aiRouting.baseUrl.help.jevCompatible",
        placeholder: "aiRouting.baseUrl.placeholder.jevCompatible",
      },
    },
  ],
  [
    "jev",
    {
      services: [
        {
          id: "typesafe",
          label: "aiProviderSettings.service.typesafe",
          host: "",
        },
      ],
      other: {
        label: "aiProviderSettings.service.otherAddress",
        help: "aiRouting.baseUrl.help.jev",
        placeholder: "aiRouting.baseUrl.placeholder.jev",
      },
    },
  ],
]);

/** Whether a provider has anything this sheet can set. */
export function hasProviderSettings(provider: string): boolean {
  return SERVICES.has(provider) || provider === VERTEX_PROVIDER;
}

// Two spellings of one address name one service: surrounding space, a trailing
// slash, and the case of scheme and host, which URLs ignore; the path keeps its
// case. Credentials and a query count, so a host carrying either is never taken
// for a listed service and rewritten without them on save.
function sameHost(a: string, b: string): boolean {
  const norm = (h: string) => {
    const trimmed = h.trim().replace(/\/+$/, "");
    try {
      const u = new URL(trimmed);
      const auth =
        u.username || u.password ? `${u.username}:${u.password}@` : "";
      return `${u.protocol}//${auth}${u.host}${u.pathname.replace(/\/+$/, "")}${u.search}`;
    } catch {
      return trimmed;
    }
  };
  return norm(a) === norm(b);
}

// No service chosen yet: a provider that cannot be reached without a host
// opens with nothing selected rather than stating a host it does not have.
const UNCHOSEN = "";

/** The service a stored host belongs to: a known one, Other, or none yet. */
export function serviceOf(provider: string, host: string): string {
  const known = SERVICES.get(provider)?.services ?? [];
  if (host.trim() === "") {
    return known.find((s) => s.host === "")?.id ?? UNCHOSEN;
  }
  return (
    known.find((s) => s.host !== "" && sameHost(s.host, host))?.id ?? OTHER
  );
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
      invalidateProviderHealth(queryClient);
    },
  });
}

// What the sheet sends: the host and location it shows, and the stored pins it
// does not — a write replaces the whole entry, so pins set elsewhere ride along
// rather than being wiped by a save that never showed them. Pins name
// OpenRouter's hosts, so they go when the provider moves off OpenRouter, which
// the server would otherwise refuse with no control here to remove them.
function settingsBody(
  host: string,
  location: string,
  openRouter: OpenRouterDraft | null,
): ProviderSettings {
  const body: ProviderSettings = {};
  if (host.trim()) body.base_url = host.trim();
  if (location) body.location = location;
  const upstream =
    openRouter && isOpenRouter(host) ? upstreamOf(openRouter) : undefined;
  if (upstream) body.upstream = upstream;
  return body;
}

/** Where a chosen service sends requests, and what to know before using it. */
function ServiceCaption({ service }: Readonly<{ service: Service }>) {
  const t = useT();
  return (
    <>
      <p className="t-caption ai-provider-host">
        {service.host
          ? t("aiProviderSettings.host.line", { host: service.host })
          : t("aiProviderSettings.host.default")}
      </p>
      {service.note && (
        <p className="t-caption">
          {t(service.note)}
          {service.noteLink && (
            <>
              {" "}
              <a href={service.noteLink} target="_blank" rel="noreferrer">
                {t("aiProviderSettings.service.learnMore")}
              </a>
            </>
          )}
        </p>
      )}
    </>
  );
}

export function ProviderSettingsForm({
  provider,
  routing,
  canManage,
  onHostChange,
}: Readonly<{
  provider: string;
  routing: Routing;
  canManage: boolean;
  /** The host the form would save, as it changes, for figures beside it. */
  onHostChange?: (host: string) => void;
}>) {
  const t = useT();
  const stored = routing.providers?.[provider] ?? {};
  const catalog = SERVICES.get(provider);
  const [service, setService] = useState(() =>
    serviceOf(provider, stored.base_url ?? ""),
  );
  // Other starts empty when the stored host is a known service's: carried
  // over it would read as a host the reader typed.
  const [typed, setTyped] = useState(() =>
    serviceOf(provider, stored.base_url ?? "") === OTHER
      ? (stored.base_url ?? "")
      : "",
  );
  const [location, setLocation] = useState(stored.location ?? "");
  const [openRouter, setOpenRouter] = useState(() => draftOf(stored.upstream));
  const save = useSetProviderSettings();
  const disabled = !canManage || save.isPending;
  const known = catalog?.services.find((s) => s.id === service);
  const host = hostOf(known, service, typed);
  const brokered = provider === "openai_compatible" && isOpenRouter(host);
  const unchosen = catalog !== undefined && service === UNCHOSEN;
  const body = settingsBody(host, location, brokered ? openRouter : null);
  const written = JSON.stringify(body);
  // What is stored, as this form would write it: Save waits for a change.
  const [saved, setSaved] = useState(written);
  useEffect(() => onHostChange?.(host), [host, onHostChange]);
  return (
    <div className="ai-provider-settings">
      {catalog && (
        <Field label={t("aiProviderSettings.service.label")}>
          {(control) => (
            <Select
              {...control}
              value={service}
              placeholder={t("aiProviderSettings.service.choose")}
              disabled={disabled}
              options={[
                ...catalog.services.map((s) => ({
                  value: s.id,
                  label: t(s.label),
                })),
                { value: OTHER, label: t(catalog.other.label) },
              ]}
              onChange={setService}
            />
          )}
        </Field>
      )}
      {known && <ServiceCaption service={known} />}
      {catalog && service === OTHER && (
        <OtherHostFields
          other={catalog.other}
          typed={typed}
          disabled={disabled}
          onChange={setTyped}
        />
      )}
      {brokered && (
        <OpenRouterSettings
          draft={openRouter}
          onChange={setOpenRouter}
          disabled={disabled}
        />
      )}
      {provider === VERTEX_PROVIDER && (
        <VertexLocationField
          value={location}
          disabled={disabled}
          onChange={setLocation}
        />
      )}
      <div className="ai-provider-settings-foot">
        <Button
          variant="primary"
          pending={save.isPending}
          disabled={!canManage || unchosen || written === saved}
          reason={canManage ? undefined : t("aiProviderKeys.adminOnly")}
          onClick={() =>
            save.mutate(
              { provider, settings: body },
              { onSuccess: () => setSaved(written) },
            )
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

/** Where the chosen service sends requests: its own host, or the one typed. */
function hostOf(
  known: Service | undefined,
  service: string,
  typed: string,
): string {
  if (known) return known.host;
  return service === OTHER ? typed : "";
}

/** The host an admin types when no listed service is theirs. */
function OtherHostFields({
  other,
  typed,
  disabled,
  onChange,
}: Readonly<{
  other: OtherHost;
  typed: string;
  disabled: boolean;
  onChange: (host: string) => void;
}>) {
  const t = useT();
  return (
    <>
      <Field label={t("aiRouting.baseUrl.label")} hint={t(other.help)}>
        {(control) => (
          <TextInput
            {...control}
            value={typed}
            disabled={disabled}
            placeholder={t(other.placeholder)}
            onChange={(e) => onChange(e.target.value)}
          />
        )}
      </Field>
      <a
        className="t-caption"
        href={HOST_GUIDE}
        target="_blank"
        rel="noreferrer"
      >
        {t("aiProviderSettings.host.guide")}
      </a>
    </>
  );
}
