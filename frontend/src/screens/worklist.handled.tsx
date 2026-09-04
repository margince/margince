// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the product did on this reader's behalf.
//
// Every other panel on this page asks for something. This one asks for nothing:
// it is the receipt a reader checks, and the reason the acts above it are safe
// to take at all. A product that acts autonomously and never says what it acted
// on is asking for trust it has not earned.
//
// NO VERBS, and that is the point rather than an omission. A receipt carries no
// action because the work is already done — a row here offering "complete" would
// ask the reader to redo it, on the one surface that exists to tell them they
// need not.
//
// FOLDED SHUT. It sits below the day's own work because a reader opens this page
// to find what to do next, and a list of what is finished answers a different
// question — one worth having and not worth leading with.

import { DataTable, Disclosure } from "../design-system/atoms";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { type Receipt, useHandledForYou } from "./worklist.queries";

export function HandledForYouPanel() {
  const t = useT();
  const { locale } = useLocale();
  // The READER's own zone. A receipt says when something happened to them, and
  // an instant rendered in UTC asks them to do the arithmetic.
  const zone = viewerZone();
  const handled = useHandledForYou();
  const state = handled.isPending
    ? "loading"
    : handled.isError
      ? "failed"
      : "ready";
  return (
    <Disclosure summary={t("worklist.handled.title")}>
      <SurfaceState
        state={state}
        emptyLabel={t("worklist.handled.empty")}
        loadingLabel={t("worklist.handled.loading")}
        detail={{ onRetry: () => void handled.refetch() }}
      >
        {/* The rows, where the read gave any. A body without them is a response
            this panel cannot read rather than a quiet day, and mapping over the
            absence would take the page down with a type error. */}
        {handled.data?.receipts && (
          <>
            <DataTable
              label={t("worklist.handled.title")}
              rows={handled.data.receipts}
              rowKey={(row: Receipt) => row.id}
              columns={[
                {
                  key: "summary",
                  header: t("worklist.handled.what"),
                  render: (row: Receipt) => row.summary,
                },
                {
                  key: "subject",
                  header: t("worklist.handled.about"),
                  // Only where the act named a record. Not every approval is
                  // about one, and an absent subject is a real state — the row
                  // reads as its summary alone rather than inventing something
                  // to point at.
                  render: (row: Receipt) =>
                    row.subject?.label ?? t("worklist.handled.noRecord"),
                },
                {
                  key: "when",
                  header: t("worklist.handled.when"),
                  render: (row: Receipt) =>
                    formatDateTime(row.occurred_at, locale, zone),
                },
              ]}
            />
            {/* A bounded read is not the whole of what was done. A reader who
                took this list for everything would close the page believing
                they had seen it all, which is the one thing a receipt surface
                must not cause. */}
            {handled.data.truncated && (
              <p className="t-caption">{t("worklist.handled.truncated")}</p>
            )}
          </>
        )}
      </SurfaceState>
    </Disclosure>
  );
}
