import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import {
  Badge,
  Button,
  EmptyState,
  SectionHeader,
} from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable } from "../../design-system/datatable";
import { LoadMoreButton, QueryStates } from "../../design-system/querystates";
import { formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { type Translator, useLocale, useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import type {
  WebhookDelivery,
  WebhookDeliveryListResponse,
  WebhookDeliveryStatus,
  WebhookSubscription,
} from "./webhooks";

// The one delivery-status to Badge tone mapping. The deliveries panel and the
// stories read it, so the tone rules are spelled once.
export function webhookStatusBadge(
  status: WebhookDeliveryStatus,
): "success" | "info" | "warning" | "danger" {
  switch (status) {
    case "delivered":
      return "success";
    case "dead_lettered":
      return "danger";
    case "retrying":
      return "warning";
    // Not "danger": nothing failed. The subscriber's endpoint is fine and the
    // record left their sight, so this is a stop and not a fault. An operator
    // should not go looking for a broken endpoint.
    case "visibility_revoked":
      return "warning";
    // `info` and not the brand accent: a delivery nobody has attempted yet is
    // work in flight. Green beside the real `delivered` row reads as a second
    // way of saying it landed.
    case "pending":
      return "info";
  }
}

// The deliveries list endpoint takes `limit` and no cursor, so it never hands
// back a usable `next_cursor` (the handler answers `page.has_more` alone).
// "Load more" asks again for a bigger page instead of inventing a cursor.
// `has_more` still drives LoadMoreButton's visibility.
const DELIVERIES_PAGE_SIZE = 20;

// The contract caps `limit` at 200 (components/parameters/Limit). "Load more"
// grows the page toward that ceiling and stops there. A contract-invalid 220
// would be rejected or clamped to the default, shrinking the page on screen.
const DELIVERIES_MAX_LIMIT = 200;

function useWebhookDeliveries(subscriptionId: string) {
  const [limit, setLimit] = useState(DELIVERIES_PAGE_SIZE);
  const query = useQuery({
    queryKey: ["webhook-deliveries", subscriptionId, limit],
    queryFn: async (): Promise<WebhookDeliveryListResponse> => {
      return unwrap(
        await api.GET("/webhook-subscriptions/{id}/deliveries", {
          params: { path: { id: subscriptionId }, query: { limit } },
        }),
      );
    },
    // Keeps the current page's rows on screen while the bigger page loads,
    // instead of flashing back to a skeleton on every "Load more" click.
    placeholderData: keepPreviousData,
  });
  return {
    query,
    loadMore: () =>
      setLimit((current) =>
        Math.min(current + DELIVERIES_PAGE_SIZE, DELIVERIES_MAX_LIMIT),
      ),
    // At the contract ceiling no larger request is valid. "Load more" stops
    // being offered even if the server still reports has_more.
    canLoadMore: limit < DELIVERIES_MAX_LIMIT,
  };
}

// Replays a parked (dead-lettered) delivery on demand. Success invalidates every
// deliveries query for this subscription, at every page size the user grew into.
// The new status then shows without a manual refetch.
function useReplayWebhookDelivery(subscriptionId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (deliveryId: string): Promise<WebhookDelivery> => {
      return unwrap(
        await api.POST(
          "/webhook-subscriptions/{id}/deliveries/{deliveryId}/replay",
          { params: { path: { id: subscriptionId, deliveryId } } },
        ),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["webhook-deliveries", subscriptionId],
      });
    },
  });
}

// The per-row replay affordance mirrors RotateSecretAction in webhooks.secret.tsx.
// A confirm guards the one side effect: a dead-lettered delivery re-attempts
// on demand instead of waiting for the next scheduled sweep.
function ReplayDeliveryAction({
  subscriptionId,
  delivery,
}: Readonly<{
  subscriptionId: string;
  delivery: WebhookDelivery;
}>) {
  const t = useT();
  const [confirming, setConfirming] = useState(false);
  const mutation = useReplayWebhookDelivery(subscriptionId);

  return (
    <>
      <Button onClick={() => setConfirming(true)} data-testid="replay-delivery">
        {t("webhooks.deliveries.replay")}
      </Button>
      <ConfirmModal
        open={confirming}
        onClose={() => setConfirming(false)}
        title={t("webhooks.deliveries.replayConfirm.title")}
        // The label names the act, not the dialog's mechanics: a generic
        // "Confirm" tells a reader nothing about what they are about to re-send.
        confirmLabel={t("webhooks.deliveries.replay")}
        onConfirm={() =>
          mutation.mutate(delivery.id, {
            onSuccess: () => setConfirming(false),
          })
        }
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
      >
        <p className="t-body">{t("webhooks.deliveries.replayConfirm.body")}</p>
      </ConfirmModal>
    </>
  );
}

// The timestamp a delivery row cares about most: when it delivered, when it
// dead-lettered, or when a row mid-retry retries next. A pending row with none
// of these renders the "no value" dash used across the app (T7).
function deliveryResolvedAt(delivery: WebhookDelivery): string | null {
  return (
    delivery.delivered_at ??
    delivery.dead_lettered_at ??
    delivery.next_retry_at ??
    null
  );
}

function deliveryColumns(
  t: Translator,
  locale: ReturnType<typeof useLocale>["locale"],
  subscriptionId: string,
  canReplay: boolean,
): {
  key: string;
  header: string;
  render: (delivery: WebhookDelivery) => ReactNode;
}[] {
  const columns = [
    {
      key: "status",
      header: t("webhooks.deliveries.column.status"),
      render: (delivery: WebhookDelivery) => (
        <Badge tone={webhookStatusBadge(delivery.status)}>
          {t(`webhooks.deliveries.status.${delivery.status}`)}
        </Badge>
      ),
    },
    {
      key: "event",
      header: t("webhooks.deliveries.column.event"),
      render: (delivery: WebhookDelivery) => <span>{delivery.event_type}</span>,
    },
    {
      key: "attempts",
      header: t("webhooks.deliveries.column.attempts"),
      render: (delivery: WebhookDelivery) => String(delivery.attempts),
    },
    {
      key: "lastStatusCode",
      header: t("webhooks.deliveries.column.lastStatusCode"),
      render: (delivery: WebhookDelivery) =>
        delivery.last_status_code != null
          ? String(delivery.last_status_code)
          : "—",
    },
    {
      key: "lastError",
      header: t("webhooks.deliveries.column.lastError"),
      render: (delivery: WebhookDelivery) => delivery.last_error ?? "—",
    },
    {
      key: "created",
      header: t("webhooks.deliveries.column.created"),
      render: (delivery: WebhookDelivery) =>
        delivery.created_at
          ? formatDateTime(delivery.created_at, locale, viewerZone())
          : "—",
    },
    {
      key: "resolved",
      header: t("webhooks.deliveries.column.resolved"),
      render: (delivery: WebhookDelivery) => {
        const at = deliveryResolvedAt(delivery);
        return at ? formatDateTime(at, locale, viewerZone()) : "—";
      },
    },
  ];
  if (!canReplay) {
    return columns;
  }
  return [
    ...columns,
    {
      key: "actions",
      header: "",
      render: (delivery: WebhookDelivery) =>
        delivery.status === "dead_lettered" ? (
          <ReplayDeliveryAction
            subscriptionId={subscriptionId}
            delivery={delivery}
          />
        ) : null,
    },
  ];
}

// Dead-lettered rows get their own table with a heading and count, so they are
// never buried among the healthy ones. The per-row danger Badge marks each one too.
export function DeliveriesPanel({
  subscription,
  canReplay,
  id,
}: Readonly<{
  subscription: WebhookSubscription;
  canReplay: boolean;
  // The region the row's toggle points `aria-controls` at, so a screen reader
  // is told what the button opens rather than only that it opened.
  id: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { query, loadMore, canLoadMore } = useWebhookDeliveries(
    subscription.id,
  );

  return (
    <div className="webhook-deliveries-panel" id={id}>
      <QueryStates query={query} pendingLabel={t("webhooks.deliveries.title")}>
        <DeliveriesBody
          response={query.data}
          subscriptionId={subscription.id}
          canReplay={canReplay}
          locale={locale}
          t={t}
          loadMoreQuery={{
            hasNextPage: (query.data?.page.has_more ?? false) && canLoadMore,
            isFetchingNextPage: query.isFetching,
            fetchNextPage: loadMore,
          }}
        />
      </QueryStates>
    </div>
  );
}

function DeliveriesBody({
  response,
  subscriptionId,
  canReplay,
  locale,
  t,
  loadMoreQuery,
}: Readonly<{
  response: WebhookDeliveryListResponse | undefined;
  subscriptionId: string;
  canReplay: boolean;
  locale: ReturnType<typeof useLocale>["locale"];
  t: Translator;
  loadMoreQuery: {
    hasNextPage: boolean;
    isFetchingNextPage: boolean;
    fetchNextPage: () => unknown;
  };
}>) {
  const deliveries = response?.data ?? [];
  if (deliveries.length === 0) {
    return <EmptyState>{t("webhooks.deliveries.empty")}</EmptyState>;
  }
  const deadLettered = deliveries.filter((d) => d.status === "dead_lettered");
  const others = deliveries.filter((d) => d.status !== "dead_lettered");
  const columns = deliveryColumns(t, locale, subscriptionId, canReplay);
  return (
    <>
      {deadLettered.length > 0 && (
        <div
          className="webhook-deliveries-group"
          data-testid="dead-letter-group"
        >
          <SectionHeader
            title={t("webhooks.deliveries.deadLetterGroup", {
              count: formatNumber(deadLettered.length, locale),
            })}
            level={3}
          />
          <DataTable
            label={t("webhooks.deliveries.deadLetterGroup", {
              count: formatNumber(deadLettered.length, locale),
            })}
            columns={columns}
            rows={deadLettered}
            rowKey={(d) => d.id}
          />
        </div>
      )}
      {others.length > 0 && (
        <div className="webhook-deliveries-group">
          {deadLettered.length > 0 && (
            <SectionHeader
              title={t("webhooks.deliveries.allGroup")}
              level={3}
            />
          )}
          <DataTable
            label={t("webhooks.deliveries.allGroup")}
            columns={columns}
            rows={others}
            rowKey={(d) => d.id}
          />
        </div>
      )}
      <LoadMoreButton query={loadMoreQuery} />
    </>
  );
}
