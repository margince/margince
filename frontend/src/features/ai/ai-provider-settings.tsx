// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  VERTEX_PROVIDER,
  VertexLocationField,
} from "../../components/vertex-location";
import { Button, Field } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Select } from "../../design-system/select";
import { lockReasonOr, useRefusalId } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { invalidateProviderHealth } from "../../lib/ai-provider-health";
import { ROUTING_KEY } from "../../lib/ai-routing-query";
import { problemMessageOf, unwrap } from "../../lib/problem";
import {
  draftOf,
  type OpenRouterDraft,
  OpenRouterSettings,
  upstreamOf,
} from "./ai-openrouter-settings";
import { isOpenRouter } from "./ai-provider-links";
import { OTHER, SERVICES, type Service } from "./ai-provider-services";
import { OtherHostFields } from "./ai-provider-settings.otherhost";

// Where a provider is reached. Set once here; every lane that binds the
// provider reads it. A known service fills its own host, so the reader picks a
// name rather than remembering a URL; Other asks for the URL with an example.

type Routing = components["schemas"]["AiRouting"];
type ProviderSettings = components["schemas"]["AiProviderSettings"];

// The providers whose sheet is only a host. They take no key, so the server's
// key list never names them and the table adds their rows itself.
export const LOCAL_PROVIDERS: readonly string[] = [...SERVICES]
  .filter(([, catalog]) => catalog.direct)
  .map(([provider]) => provider);

type KeyStatus = components["schemas"]["AiProviderKeyStatus"];

/** The key list with a keyless row for each local provider it does not name. */
export function withLocalProviders(
  listed: readonly KeyStatus[],
): readonly KeyStatus[] {
  const named = new Set(listed.map((p) => p.provider));
  const local = LOCAL_PROVIDERS.filter((p) => !named.has(p)).map(
    (provider): KeyStatus => ({
      provider,
      configured: false,
      env_var: "",
      optional: false,
      credential_kind: "api_key",
      usable: true,
    }),
  );
  return [...listed, ...local];
}

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
  if (SERVICES.get(provider)?.direct) return OTHER;
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
      unwrap(
        await api.PUT("/ai/provider-settings/{provider}", {
          params: { path: { provider: vars.provider } },
          body: vars.settings,
        }),
      );
    },
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({ queryKey: ROUTING_KEY });
      // A key test and a model list ask the provider at its host.
      queryClient.invalidateQueries({
        queryKey: ["ai-available-models", vars.provider],
      });
      return invalidateProviderHealth(queryClient);
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
  // Host and location live in the routing document, so its lock binds them.
  const refusal = lockReasonOr(
    routing.locks,
    "ai.routing",
    t,
    canManage ? undefined : t("aiProviderKeys.adminOnly"),
  );
  const reasonId = useRefusalId(refusal);
  const disabled = refusal !== undefined || save.isPending;
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
      {reasonId && (
        <p id={reasonId} className="t-caption">
          {refusal}
        </p>
      )}
      {catalog && !catalog.direct && (
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
          guide={!catalog.direct}
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
          disabled={unchosen || written === saved}
          reasonId={reasonId}
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
