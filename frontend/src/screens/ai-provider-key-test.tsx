// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";

// Whether a stored key works, asked of the vendor that issued it.
//
// The server makes the call with the key it already holds; nothing typed here
// travels. What comes back is a closed reason, never the vendor's own words,
// and a model count only where the test listed models — a broker's key
// endpoint and the decision probe pass without one.

type KeyTestResult = components["schemas"]["AiProviderKeyTestResult"];

export function useTestProviderKey() {
  return useMutation({
    // Short-lived like the key save beside it: the result describes a moment,
    // and a stale "connected" kept in cache would outlive a revoked key.
    gcTime: 0,
    mutationFn: async (vars: { provider: string }) => {
      const { data, error } = await api.POST(
        "/ai/provider-keys/{provider}/test",
        { params: { path: { provider: vars.provider } } },
      );
      if (error) {
        throwProblem(error);
      }
      if (!data) throw new Error("Provider key test unavailable");
      return data;
    },
  });
}

export function KeyTestButton({
  provider,
  test,
}: Readonly<{
  provider: string;
  test: ReturnType<typeof useTestProviderKey>;
}>) {
  const t = useT();
  return (
    <Button
      pending={test.isPending}
      busyLabel={t("aiProviderKeys.testing")}
      onClick={() => test.mutate({ provider })}
    >
      {t("aiProviderKeys.test")}
    </Button>
  );
}

// The answer, on its own line under the row: a pass says how much the vendor
// serves, a failure says which of the failures it was.
export function KeyTestOutcome({
  test,
  keyHeld,
}: Readonly<{
  test: ReturnType<typeof useTestProviderKey>;
  // Whether a key was sent: a keyless pass says the server answered, not that
  // it accepted a key nobody gave it.
  keyHeld: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (test.isError) {
    return (
      <p className="t-sub" role="status">
        <Badge tone="danger">{t("aiProviderKeys.testFailed")}</Badge>{" "}
        {problemMessageOf(test.error, t)}
      </p>
    );
  }
  if (!test.data) return null;
  const result = test.data;
  return (
    <p className="t-sub" role="status">
      {result.ok ? (
        <>
          <Badge tone="success">{t("aiProviderKeys.connected")}</Badge>{" "}
          {result.model_count === undefined
            ? t(passLine(keyHeld, result.key_confirmed !== false))
            : plural("aiProviderKeys.modelCount", result.model_count, {
                count: formatNumber(result.model_count, locale),
              })}
        </>
      ) : (
        <>
          <Badge tone="danger">{t("aiProviderKeys.testFailed")}</Badge>{" "}
          {t(keyTestReasonKey(result.provider, result.reason))}
        </>
      )}
    </p>
  );
}

// What a pass without a count proves. A keyless server accepted no key; a
// server that answered the empty decision request without refusing it may
// not have read the key at all.
function passLine(
  keyHeld: boolean,
  confirmed: boolean,
):
  | "aiProviderKeys.accepted"
  | "aiProviderKeys.answered"
  | "aiProviderKeys.unconfirmed" {
  if (!keyHeld) return "aiProviderKeys.answered";
  return confirmed ? "aiProviderKeys.accepted" : "aiProviderKeys.unconfirmed";
}

/** The sentence for why a test failed; a Vertex refusal names the role it lacks. */
export function keyTestReasonKey(
  provider: string,
  reason: KeyTestResult["reason"],
): MessageKey {
  switch (reason) {
    case "auth_failed":
      return "aiProviderKeys.reason.authFailed";
    case "permission_denied":
      return provider === "gemini_vertex"
        ? "aiProviderKeys.reason.permissionDeniedVertex"
        : "aiProviderKeys.reason.permissionDenied";
    case "rate_limited":
      return "aiProviderKeys.reason.rateLimited";
    case "no_key":
      return "aiProviderKeys.reason.noKey";
    case "no_endpoint":
      return "aiProviderKeys.reason.noEndpoint";
    case "profile_forbids":
      return "aiProviderKeys.reason.profileForbids";
    case "not_published":
      return "aiProviderKeys.reason.notPublished";
    default:
      return "aiProviderKeys.reason.unreachable";
  }
}
