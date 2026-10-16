import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Webhook } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Badge,
  Button,
  EmptyState,
  Modal,
  OverflowMenu,
  SectionHeader,
} from "../design-system/atoms";
import { CopyableText, useClipboardCopy } from "../design-system/clipboardcopy";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import {
  LoadMoreButton,
  QueryGate,
  QueryStates,
} from "../design-system/querystates";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import {
  problemCode,
  problemMessageOf,
  throwProblem,
  unwrap,
} from "../lib/problem";
import { useMe } from "../lib/session";
import { ArchiveAction } from "./archive";
import { WebhookSubscriptionDrawer } from "./webhookdrawer";
import "./webhooks.css";

// Settings → Integrations (B-E10.14): the subscription list for outbound
// webhooks. The list wire (WebhookSubscription) carries no per-item delivery
// health — that lives on the separate deliveries sub-resource, out of this
// card's scope — so the health line here renders only what the list itself
// is honest about: state, the subscribed event set, and last-updated. A
// deployment with no signing key answers 503 webhooks_not_configured; that
// is a deliberate, documented feature-off state, never an error.

type WebhookSubscription = components["schemas"]["WebhookSubscription"];
type WebhookDelivery = components["schemas"]["WebhookDelivery"];
type WebhookDeliveryStatus = WebhookDelivery["status"];
type WebhookSubscriptionCreated =
  components["schemas"]["WebhookSubscriptionCreated"];
type WebhookDeliveryListResponse =
  components["schemas"]["WebhookDeliveryListResponse"];

// The shared delivery-status → Badge tone mapping: kept here, next to the
// subscription list it health-summarizes, so the deliveries panel reuses the
// same spelling rather than re-deriving its own tone rules per status.
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
    // Not "danger": nothing failed. The subscriber's own endpoint is fine and
    // the record simply left their sight, so this is a stop rather than a
    // fault, and an operator reading the list should not go looking for a
    // broken endpoint.
    case "visibility_revoked":
      return "warning";
    // `info` and not the brand accent: a delivery nobody has attempted yet is
    // work in flight, and green beside the real `delivered` row read as a
    // second way of saying it landed.
    case "pending":
      return "info";
  }
}

// deliveryEnabled reports whether this deployment has a signing key: the list
// works either way (subscriptions are inspectable), but create/rotate/replay
// need the key and answer 503 otherwise — so the card gates its mutating
// controls on the list response's delivery_enabled flag, not on whether the
// list happened to load (which never signals the missing key).
type SubscriptionsResult = {
  deliveryEnabled: boolean;
  data: WebhookSubscription[];
};

function useWebhookSubscriptions() {
  return useQuery({
    queryKey: ["webhook-subscriptions"],
    queryFn: async (): Promise<SubscriptionsResult> => {
      const { data, error, response } = await api.GET(
        "/webhook-subscriptions",
        { params: { query: {} } },
      );
      // "Not configured on this deployment" is the specific 503 whose
      // RFC-7807 code is webhooks_not_configured — key on the code, not the
      // bare status, so a transient dependency 503 (DB down behind the API)
      // still surfaces as an error card instead of the calm not-enabled state.
      if (
        response.status === 503 &&
        problemCode(error) === "webhooks_not_configured"
      ) {
        return { deliveryEnabled: false, data: [] };
      }
      if (error || !response.ok) {
        throwProblem(error);
      }
      return { deliveryEnabled: data.delivery_enabled, data: data.data };
    },
  });
}

// Archive stops all delivery (DELETE, no If-Match — mirrors products.tsx/
// contacts.tsx's ArchiveAction usage: archiving isn't a concurrent-edit hazard
// the way a field patch is).
async function archiveWebhookSubscription(
  subscription: WebhookSubscription,
): Promise<WebhookSubscription> {
  const data = unwrap(
    await api.DELETE("/webhook-subscriptions/{id}", {
      params: { path: { id: subscription.id } },
    }),
  );
  return data ?? subscription;
}

// Rotate-secret: a Button + the shared ConfirmModal chrome (mirrors offers.tsx's
// RejectOfferAction) guarding the one irreversible side effect — the OLD
// secret stops verifying the moment this succeeds. The new secret is handed
// up to the card so it reuses the SAME SecretRevealModal a create shows.
function RotateSecretAction({
  subscription,
  onRotated,
}: Readonly<{
  subscription: WebhookSubscription;
  onRotated: (secret: string) => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const mutation = useMutation({
    mutationFn: async (): Promise<WebhookSubscriptionCreated> => {
      return unwrap(
        await api.POST("/webhook-subscriptions/{id}/rotate-secret", {
          params: { path: { id: subscription.id } },
        }),
      );
    },
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: ["webhook-subscriptions"] });
      queryClient.invalidateQueries({
        queryKey: ["webhook-subscription", subscription.id],
      });
      setConfirming(false);
      onRotated(created.signing_secret);
    },
  });

  return (
    <>
      {/* The old secret stops verifying the moment this succeeds, and there is
          no way back to it — so the control reads as destructive and the
          confirm's own button does too. It rendered as a plain default button
          beside Edit, at exactly the weight of a reversible change, and its
          confirm borrowed "Confirm" from the deals namespace, which named the
          dialog's mechanics rather than the act being confirmed. */}
      <Button
        variant="danger"
        onClick={() => setConfirming(true)}
        data-testid="rotate-webhook-secret"
      >
        {t("webhooks.rotate")}
      </Button>
      <ConfirmModal
        open={confirming}
        onClose={() => setConfirming(false)}
        title={t("webhooks.rotateConfirm.title")}
        confirmLabel={t("webhooks.rotate")}
        confirmVariant="danger"
        onConfirm={() => mutation.mutate()}
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
      >
        <p className="t-body">{t("webhooks.rotateConfirm.body")}</p>
      </ConfirmModal>
    </>
  );
}

// The one-time secret reveal: shown immediately after a successful create,
// gone the moment this modal closes — the secret lives only in the parent's
// local state (never react-query cache, never re-derivable from a list/get
// response) and there is no way back to it once dismissed.
function SecretRevealModal({
  secret,
  onClose,
}: Readonly<{ secret: string; onClose: () => void }>) {
  const t = useT();
  const headingId = useId();
  const copy = useClipboardCopy(secret, {
    copy: t("webhooks.secret.copy"),
    copied: t("webhooks.secret.copied"),
    remedy: t("webhooks.secret.copyFailed"),
  });

  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="confirm">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("webhooks.secret.title")}
      </Heading>
      {/* One stack owns every interval in this dialog, so the warning, the
          secret and the copy attempt do not each set a margin of their own. */}
      <div className="form-stack">
        <p>{t("webhooks.secret.warning")}</p>
        <CopyableText
          text={secret}
          label={t("webhooks.secret.title")}
          testId="webhook-signing-secret"
        />
        {copy.notice}
      </div>
      {/* Dismissing destroys the only copy of the secret, which no read can
          re-derive: Copy is primary, and Done stays available before a copy
          with a caution saying what leaving costs. */}
      {!copy.copied && (
        // ds:ignore a caution, not a refusal
        <p className="webhook-secret-caution">
          {t("webhooks.secret.leaveWarning")}
        </p>
      )}
      <div className="actions">
        <Button onClick={onClose}>{t("webhooks.secret.done")}</Button>
        <Button variant="primary" onClick={copy.copy}>
          {copy.label}
        </Button>
      </div>
    </Modal>
  );
}

function subscriptionStateTone(
  state: WebhookSubscription["state"],
): "success" | "warning" {
  return state === "active" ? "success" : "warning";
}

function NotConfiguredState() {
  const t = useT();
  return <EmptyState>{t("webhooks.notConfigured")}</EmptyState>;
}

// Deliveries + dead-letter inspection (Task 10, B-E10.13c/B-E10.15): the
// list endpoint has no cursor query param — only `limit` — so it never hands
// back a usable `next_cursor` (confirmed: the handler always answers
// `page.has_more` alone). "Load more" is therefore honestly implemented as
// re-asking for a BIGGER page rather than fabricating a cursor the contract
// doesn't have: `has_more` still drives LoadMoreButton's visibility
// truthfully, it just grows the page instead of paging past it.
const DELIVERIES_PAGE_SIZE = 20;
// The contract caps `limit` at 200 (components/parameters/Limit). "Load more"
// grows the page toward that ceiling and stops there — never requesting a
// contract-invalid 220 (which the server would reject / silently clamp back to
// its default, shrinking the page the user is looking at).
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
    // Once the page has grown to the contract ceiling there is no valid larger
    // request to make, so "Load more" must stop being offered even if the
    // server still reports has_more.
    canLoadMore: limit < DELIVERIES_MAX_LIMIT,
  };
}

// Replays a parked (dead-lettered) delivery on demand, then invalidates
// every deliveries query for this subscription (across every page-size the
// user has grown into) so the replayed row's refreshed status is visible
// without a manual refetch.
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

// The per-row replay affordance: a Button + the shared ConfirmModal chrome
// (mirrors RotateSecretAction above) guarding the one side effect — a
// dead-lettered delivery re-attempts on demand rather than waiting for the
// next scheduled sweep.
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
        // The act, not the dialog's mechanics: "Confirm" was borrowed from the
        // deals namespace and told a reader nothing about what they were about
        // to re-send.
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

// The terminal/next timestamp a delivery row cares about most: when it
// delivered, when it dead-lettered, or (still mid-retry) when it retries
// next — whichever of those the row actually carries. Falls back to the em
// dash the rest of this codebase already uses for "no value" (T7: honest
// about what a pending-with-no-terminal-timestamp-yet row can show).
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

// The dead-letter-grouped delivery list: dead-lettered rows read as a
// visually distinct table (their own heading + count), never buried
// undifferentiated among the healthy ones — this IS the "grouped/marked"
// requirement, on top of the per-row status Badge (danger tone) that already
// marks them individually.
function DeliveriesPanel({
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

function SubscriptionRow({
  subscription,
  canEdit,
  canArchive,
  onRotated,
}: Readonly<{
  subscription: WebhookSubscription;
  // Editing the config, rotating the signing secret and replaying a delivery
  // are all webhook_subscription:update; archiving is the delete. They are
  // separate grants, so they are separate props rather than one "manage" flag
  // that would show an archive button to a role holding only update.
  canEdit: boolean;
  canArchive: boolean;
  onRotated: (secret: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const [showDeliveries, setShowDeliveries] = useState(false);
  const [editing, setEditing] = useState(false);
  const deliveriesId = useId();
  return (
    // Two rows, not one: the subscription is a DECISION — what it targets on
    // the left, what it is set to on the right — and its attempt log is the
    // SUBJECT rather than an answer to that question, so it takes the full
    // width below instead of being squeezed into the right column. They are
    // siblings in the card's one `SettingList`, which is what rules between
    // them.
    <>
      <SettingRow
        label={
          <span className="webhook-target" title={subscription.target_url}>
            {subscription.target_url}
          </span>
        }
        // The verbs sit under the target, so a long URL keeps the row's width.
        description={
          <span className="webhook-row-description">
            {subscription.updated_at
              ? t("webhooks.updated", {
                  date: formatDateTime(
                    subscription.updated_at,
                    locale,
                    viewerZone(),
                  ),
                })
              : undefined}
            <span className="webhook-row-actions">
              {/* Not `Disclosure`: `<details>` renders its children
                whether or not it is open, and the panel behind this one fetches
                a subscription's deliveries on mount — every row on the card
                would issue that read on page load to draw a section nobody
                opened. The accessibility half of a disclosure is what was
                actually missing, so the button carries it: what it controls,
                and whether it is open. */}
              <Button
                data-testid="view-deliveries"
                aria-expanded={showDeliveries}
                aria-controls={deliveriesId}
                onClick={() => setShowDeliveries((prev) => !prev)}
              >
                {showDeliveries
                  ? t("webhooks.deliveries.hide")
                  : t("webhooks.deliveries.show")}
              </Button>
              {canEdit && (
                <Button
                  data-testid="edit-webhook-subscription"
                  onClick={() => setEditing(true)}
                >
                  {t("webhooks.edit")}
                </Button>
              )}
              {/* The two irreversible verbs behind the overflow, the same
                treatment the provider card's disconnect/delete pair gets:
                rotating destroys the secret every receiver is verifying with,
                archiving stops every delivery. Beside Edit they were four
                buttons on one line at equal weight, two of them red, and the
                row did not even wrap. */}
              {(canEdit || canArchive) && (
                <OverflowMenu label={t("record.moreActions")}>
                  {canEdit && (
                    <RotateSecretAction
                      subscription={subscription}
                      onRotated={onRotated}
                    />
                  )}
                  {canArchive && (
                    <ArchiveAction
                      label={t("webhooks.archive")}
                      confirmText={t("webhooks.archiveConfirm")}
                      archivedMessage={t("webhooks.archiveDone")}
                      invalidate="webhook-subscriptions"
                      recordKey="webhook-subscription"
                      onArchived={() => {}}
                      archive={() => archiveWebhookSubscription(subscription)}
                    />
                  )}
                </OverflowMenu>
              )}
            </span>
          </span>
        }
        // The state and the subscribed event set together ARE the current
        // answer, and no control on the row shows either: Edit opens a dialog
        // and the deliveries toggle opens a log.
        value={
          <span className="webhook-answer">
            <Badge tone={subscriptionStateTone(subscription.state)}>
              {t(`webhooks.state.${subscription.state}`)}
            </Badge>
            {subscription.body_template && (
              <Badge tone="default">{t("webhooks.row.customBody")}</Badge>
            )}
            {(subscription.custom_headers?.length ?? 0) > 0 && (
              <Badge tone="default">
                {plural(
                  "webhooks.row.headers",
                  subscription.custom_headers.length,
                  {
                    count: formatNumber(
                      subscription.custom_headers?.length ?? 0,
                      locale,
                    ),
                  },
                )}
              </Badge>
            )}
            {subscription.event_types.map((eventType) => (
              <Badge key={eventType} tone="accent">
                {eventType}
              </Badge>
            ))}
          </span>
        }
        control={null}
      />
      {editing && (
        <WebhookSubscriptionDrawer
          subscription={subscription}
          onClose={() => setEditing(false)}
        />
      )}
      {showDeliveries && (
        <SettingRow
          label={t("webhooks.deliveries.title")}
          layout="stack"
          control={
            <DeliveriesPanel
              subscription={subscription}
              canReplay={canEdit}
              id={deliveriesId}
            />
          }
        />
      )}
    </>
  );
}

export function WebhooksCard() {
  const t = useT();
  // Three grants, three affordances. The seeded matrix gives admin and ops all
  // three and every other role none, so today they move together — but they are
  // independent columns in role.permissions, and an operator who narrows one is
  // entitled to a UI that follows.
  const canCreate = useCanWrite("webhook_subscription", "create");
  const canEdit = useCanWrite("webhook_subscription", "update");
  const canArchive = useCanWrite("webhook_subscription", "delete");
  // The probe itself, not just its answer: all three grants read false while
  // /me is in flight, so branching on their absence alone would flash the
  // read-only line at an operator on every load.
  const me = useMe();
  const query = useWebhookSubscriptions();
  const [creating, setCreating] = useState(false);
  const [revealedSecret, setRevealedSecret] = useState<string | null>(null);
  // Gated on the deployment actually being configured (never on the CURRENT
  // list happening to be empty) — the button lives outside QueryGate's
  // render-prop specifically so the very first subscription (the empty-list
  // case) is still creatable; QueryGate's `empty` branch renders the shared
  // EmptyState in place of `children`, which would otherwise swallow a
  // button nested inside it.
  const deliveryEnabled = query.data?.deliveryEnabled === true;
  const canCreateHere = canCreate && deliveryEnabled;
  const deliveryOff = query.isSuccess && !deliveryEnabled;
  // Two different absences can empty this card of every write control, and
  // they are two different rows of the design-system doctrine: no deployment
  // signing key is a PRECONDITION an operator can fix, which NotConfiguredState
  // already names along with what would make it live, while holding none of the
  // three grants is a PERMISSION, which needs saying or the missing buttons
  // read as a bug.
  //
  // When both hold, the deployment fact wins and this line stays quiet.
  // Delivery being off withholds these controls from EVERY seat, an admin
  // included, so it is genuinely the reason they are absent for this reader
  // too — pinning them on the seat instead would tell a rep their role is what
  // stopped them, and send them to ask for a grant that changes nothing. The
  // permission is not lost, only deferred: configure a key and it becomes the
  // live constraint, and this line appears.
  const showReadOnlyPosture =
    me.isSuccess && deliveryEnabled && !canCreate && !canEdit && !canArchive;

  return (
    // No per-card bottom margin: the tab owns the rhythm between its cards, and
    // one card paying for its own gap is how a page ends up with two different
    // ones depending on which card happens to be above.
    <Panel
      title={t("webhooks.title")}
      titleAction={
        canCreateHere ? (
          <Button
            variant="primary"
            data-testid="new-webhook-subscription"
            onClick={() => setCreating(true)}
          >
            <Webhook aria-hidden /> {t("webhooks.new")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        {/* Off, the not-enabled line says what webhooks wait for. */}
        {!deliveryOff && <PanelIntro>{t("webhooks.sub")}</PanelIntro>}
        {/* Outside QueryGate for the same reason the create button is: its
            `empty` branch replaces `children` wholesale, and the posture is most
            needed precisely when the list is empty — a seat that can neither add
            the first subscription nor be told why would read the empty card as
            the whole story. */}
        {showReadOnlyPosture && (
          <PanelIntro>{t("webhooks.readOnly")}</PanelIntro>
        )}
        {/* No signing key: delivery is off, so mutating controls are withheld
            and a not-enabled note explains why. It sits outside the gate too —
            it is a fact about the DEPLOYMENT, and the list's own state has no
            bearing on it. Existing subscriptions still render read-only (write
            grants forced false) so their config and delivery health stay
            inspectable. */}
        {deliveryOff && <NotConfiguredState />}
        {/* One body, not two stacked: the roster opens under the sentence that
            introduces it rather than across a seam that reads as a missing
            element, and the interval between subscriptions belongs to the
            `SettingList` — which is the whole reason the rows no longer space
            themselves. */}
        <QueryGate
          pendingLabel={t("webhooks.title")}
          query={query}
          empty={(result) => result.deliveryEnabled && result.data.length === 0}
        >
          {(result) => (
            <SettingList>
              {result.data.map((subscription) => (
                <SubscriptionRow
                  key={subscription.id}
                  subscription={subscription}
                  canEdit={canEdit && result.deliveryEnabled}
                  canArchive={canArchive && result.deliveryEnabled}
                  onRotated={setRevealedSecret}
                />
              ))}
            </SettingList>
          )}
        </QueryGate>
      </PanelBody>
      {canCreateHere && creating && (
        <WebhookSubscriptionDrawer
          onClose={() => setCreating(false)}
          onCreated={setRevealedSecret}
        />
      )}
      {revealedSecret && (
        <SecretRevealModal
          secret={revealedSecret}
          onClose={() => setRevealedSecret(null)}
        />
      )}
    </Panel>
  );
}
