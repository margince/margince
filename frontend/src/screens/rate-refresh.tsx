// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import "./rates.css";

// Asking the product to go and re-read a price sheet from its sources.
//
// Its own module because TWO screens offer it — the currency sheet on the
// organization page and the model sheet beside the lanes it prices — and a
// screen must not reach into another screen for a control. It is not a
// design-system primitive either: it owns a mutation, an endpoint and its own
// copy, and the design system's rule is that no copy lives in a primitive. What
// it is, is one screen-tier capability with two callers, so it lives at screen
// tier with a file of its own.

/**
 * RefreshFromSources enqueues an async refresh that stages proposals into the
 * approvals inbox (the job runs in the background — nothing to poll here). The
 * button stays available so an admin can trigger another refresh; success and
 * failure both surface next to it.
 */
export function RefreshFromSources({
  path,
}: Readonly<{
  path: "/fx-rates/propose-refresh" | "/ai-model-rates/propose-refresh";
}>) {
  const t = useT();
  const refresh = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST(path);
      if (error) {
        throwProblem(error);
      }
    },
  });
  return (
    <span className="rates-refresh">
      <Button
        variant="ghost"
        small
        onClick={() => refresh.mutate()}
        disabled={refresh.isPending}
      >
        {t("settings.rates.refresh")}
      </Button>
      {refresh.isSuccess ? (
        <span className="t-small" role="status">
          {t("settings.rates.refreshEnqueued")}
        </span>
      ) : null}
      {/* The failure is spoken, and the retry is the button it sits beside —
          which stays enabled, so the reader does not need a second control that
          would do the same thing. */}
      {refresh.isError ? (
        <span className="rates-error" role="alert">
          {problemMessageOf(refresh.error, t)}
        </span>
      ) : null}
    </span>
  );
}
