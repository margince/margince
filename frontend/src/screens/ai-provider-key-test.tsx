// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { DECISION_PROVIDERS } from "./ai-routing-fields";
import { problemMessageOf, throwProblem } from "./common";

// Whether a stored key works, asked of the vendor that issued it.
//
// The server makes the call with the key it already holds; nothing typed here
// travels. What comes back is a closed reason, never the vendor's own words.

type KeyTestResult = components["schemas"]["AiProviderKeyTestResult"];

// A decision server publishes no model list, and listing is what the test
// does — so a Test on these rows could only ever answer "not published".
const UNTESTABLE: ReadonlySet<string> = new Set(DECISION_PROVIDERS);

export function keyTestable(provider: string): boolean {
  return !UNTESTABLE.has(provider);
}

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
}: Readonly<{ test: ReturnType<typeof useTestProviderKey> }>) {
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
          {plural("aiProviderKeys.modelCount", result.model_count ?? 0, {
            count: formatNumber(result.model_count ?? 0, locale),
          })}
        </>
      ) : (
        <>
          <Badge tone="danger">{t("aiProviderKeys.testFailed")}</Badge>{" "}
          {keyTestReason(result.reason, t)}
        </>
      )}
    </p>
  );
}

function keyTestReason(
  reason: KeyTestResult["reason"],
  t: ReturnType<typeof useT>,
): string {
  switch (reason) {
    case "auth_failed":
      return t("aiProviderKeys.reason.authFailed");
    case "rate_limited":
      return t("aiProviderKeys.reason.rateLimited");
    case "no_key":
      return t("aiProviderKeys.reason.noKey");
    case "no_endpoint":
      return t("aiProviderKeys.reason.noEndpoint");
    case "profile_forbids":
      return t("aiProviderKeys.reason.profileForbids");
    case "not_published":
      return t("aiProviderKeys.reason.notPublished");
    default:
      return t("aiProviderKeys.reason.unreachable");
  }
}
