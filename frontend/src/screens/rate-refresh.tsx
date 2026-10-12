// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import { unwrap } from "./common";
import "./rates.css";

/** The sheet this product re-reads from its source through an approval. */
type RefreshPath = "/fx-rates/propose-refresh";

// Asking the product to go and re-read the currency sheet from its source.
//
// Its own module because it is a mutation, an endpoint and its own copy, and
// the design system's rule is that no copy lives in a primitive. The model
// sheet has its own control (`rate-catalogue-refresh.tsx`): its answer arrives
// with the response rather than in the approvals inbox.

/**
 * RefreshFromSources enqueues an async refresh that stages proposals into the
 * approvals inbox (the job runs in the background — nothing to poll here). The
 * button stays available so an admin can trigger another refresh; success and
 * failure both surface next to it.
 */
export function RefreshFromSources({ path }: Readonly<{ path: RefreshPath }>) {
  const t = useT();
  const refresh = useMutation({
    // The path travels as a VARIABLE rather than closing over the render that
    // drew the button. React Query re-arms a mutation's options in a passive
    // effect, so a click landing before that effect runs would carry the
    // previous render's closure — and `mutation-variable-coverage.test.ts`
    // walks the TSX for exactly this shape.
    mutationFn: async (target: RefreshPath) => {
      unwrap(await api.POST(target));
    },
  });
  return (
    <span className="rates-refresh">
      <Button
        variant="ghost"
        onClick={() => refresh.mutate(path)}
        // `pending`, not `disabled`. A natively disabled button drops the focus
        // of the reader who just pressed it and announces nothing; `pending` is
        // what this design system reserves for a write in flight, and it blocks
        // the repeat press without taking the focus away.
        pending={refresh.isPending}
      >
        {t("settings.rates.refresh")}
      </Button>
      {refresh.isSuccess ? (
        <span className="t-caption" role="status">
          {t("settings.rates.refreshEnqueued")}
        </span>
      ) : null}
      {/* The failure is spoken, and the retry is the button it sits beside —
          which stays enabled, so the reader does not need a second control that
          would do the same thing. */}
      <ErrorLine error={refresh.error} inline />
    </span>
  );
}
