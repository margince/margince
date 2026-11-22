// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { VertexLocationField } from "../../components/vertex-location";
import { Field, TextInput } from "../../design-system/atoms";
import { ServiceAccountKeyField } from "../../design-system/serviceaccountkeyfield";
import { lockFor } from "../../format/settinglocks";
import { type Translator, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useRouting } from "../../lib/ai-routing-query";
import { useCan } from "../../lib/capability";
import { MODEL_SETUPS, type ModelSetupId } from "../../lib/modelsetups.gen";
import { unwrap } from "../../lib/problem";
import type { SetupProvider } from "../../lib/setup-providers";

// The model step's credential and where it is bound: the key (pasted, or a
// service-account file for Vertex), the location Vertex is bound at, and the
// write that binds every lane under the choice's own profile.

// A boot source that sets the routing owns the binding, so the step stores the
// key and leaves the routing as that source set it.
function useRoutingLock() {
  const t = useT();
  const canRoute = useCan("ai_routing", "read");
  return lockFor(useRouting(canRoute).data?.routing.locks, "ai.routing", t);
}

export function useBindModels() {
  const locked = useRoutingLock().disabled;
  const queryClient = useQueryClient();
  const bind = useMutation({
    // Same reasoning as the provider-key mutation: nothing here is a secret,
    // but the two settle together and a stale binding on screen after a
    // success is the same confusion.
    gcTime: 0,
    mutationFn: async (vars: {
      provider: string;
      baseUrl?: string;
      location?: string;
      // Where the embedder is bound when the chat location serves none.
      embedLocation?: string;
      profile: SetupProvider["profile"];
      // The graded preset to bind tier by tier; absent when the reader chose
      // one model for every tier.
      setup?: ModelSetupId;
      chatModel: string;
      embedModel: string;
      routingLocked?: boolean;
    }) => {
      if (vars.routingLocked) {
        return;
      }
      const host = vars.baseUrl ? { base_url: vars.baseUrl } : {};
      const embedLocation = vars.embedLocation ?? vars.location;
      unwrap(
        await api.PUT("/ai/routing", {
          body: {
            // The choice's own profile, not a question. cloud_frontier for a vendor
            // bound to keep no EU promise (the server refuses an unpinned broker under
            // eu_hosted), eu_hosted for Vertex at an EU location. A first-time admin
            // cannot yet answer a location ladder before binding anything.
            profile: vars.profile,
            tiers: chatTiers(vars, host),
            embeddings: {
              provider: vars.provider,
              model: vars.embedModel,
              ...host,
              ...(embedLocation ? { location: embedLocation } : {}),
            },
          },
        }),
      );
    },
    // The assistant profile follows the binding at once. The setup report does
    // not: re-reading it moves the screen on, and
    // the binding is the moment the ignition marks. The refetch waits for the
    // reader to press past it (`onDone`); a reload mid-sequence finds
    // `ai_models` configured and loses nothing but the ceremony.
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["assistant-profile"] }),
  });
  // The lock travels as a variable, read by the render that drew the press.
  return {
    ...bind,
    mutate: (
      vars: Omit<Parameters<typeof bind.mutate>[0], "routingLocked">,
      options?: Parameters<typeof bind.mutate>[1],
    ) => bind.mutate({ ...vars, routingLocked: locked }, options),
  };
}

type ChatTiers = components["schemas"]["AiRouting"]["tiers"];

/**
 * Every chat tier, bound. A tier left unbound degrades honestly at runtime, but
 * an onboarding that bound only some would have the product answer for one task
 * and refuse another with no way for the reader to tell which they configured.
 * The graded preset binds each tier to its own model, at the location the
 * reader chose; the advanced choice puts the one model on all four.
 */
export function chatTiers(
  vars: Readonly<{
    provider: string;
    location?: string;
    setup?: ModelSetupId;
    chatModel: string;
  }>,
  host: Readonly<{ base_url?: string }>,
): ChatTiers {
  const at = vars.location ? { location: vars.location } : {};
  if (vars.setup) {
    const tiers: ChatTiers = {};
    for (const [tier, binding] of Object.entries(
      MODEL_SETUPS[vars.setup].routing.tiers,
    )) {
      tiers[tier] = { ...binding, ...at };
    }
    return tiers;
  }
  const binding = {
    provider: vars.provider,
    model: vars.chatModel,
    ...host,
    ...at,
  };
  return {
    local_small: binding,
    cheap_cloud: binding,
    premium: binding,
    frontier: binding,
  };
}

// A choice that keeps processing in the EU says so in its name: it is the one
// thing that sets it apart from the plain Gemini choice above it.
export function choiceLabel(preset: SetupProvider, t: Translator): string {
  return preset.profile === "eu_hosted"
    ? `${preset.label} (${t("firstRun.ai.euResidency")})`
    : preset.label;
}

function keyFieldError(
  attempted: boolean,
  secret: string,
  refusal: MessageKey | undefined,
  t: Translator,
): string | undefined {
  if (attempted && secret.trim() === "") {
    return t("firstRun.needed");
  }
  return refusal ? t(refusal) : undefined;
}

/** The credential the chosen vendor takes, and where Vertex is bound. */
export function AiKeyFields({
  preset,
  secret,
  refusal,
  attempted,
  location,
  disabled,
  onSecret,
  onLocation,
}: Readonly<{
  preset: SetupProvider;
  secret: string;
  refusal: MessageKey | undefined;
  attempted: boolean;
  location: string;
  disabled: boolean;
  onSecret: (secret: string) => void;
  onLocation: (location: string) => void;
}>) {
  const t = useT();
  const routingLock = useRoutingLock();
  return (
    <>
      {routingLock.disabled && (
        <p className="t-caption">{routingLock.reason}</p>
      )}
      {preset.credential === "service_account" ? (
        <ServiceAccountKeyField
          value={secret}
          disabled={disabled}
          hint={t("firstRun.ai.keyHint")}
          error={keyFieldError(attempted, secret, refusal, t)}
          onChange={onSecret}
        />
      ) : (
        <Field
          label={t("firstRun.ai.key")}
          hint={t("firstRun.ai.keyHint")}
          error={keyFieldError(attempted, secret, refusal, t)}
        >
          {(control) => (
            <TextInput
              {...control}
              // A password field so the browser does not offer to remember a
              // credential this app never stores client-side, and a screenshare
              // does not carry it.
              type="password"
              autoComplete="off"
              value={secret}
              disabled={disabled}
              onChange={(e) => onSecret(e.target.value)}
            />
          )}
        </Field>
      )}
      {preset.location !== undefined && (
        <VertexLocationField
          value={location}
          disabled={disabled}
          noKeyHint={t("firstRun.ai.locationBeforeKey")}
          onChange={onLocation}
        />
      )}
    </>
  );
}
