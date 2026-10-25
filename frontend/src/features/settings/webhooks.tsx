import { useQuery } from "@tanstack/react-query";
import { Webhook } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ArchiveAction } from "../../components/archive";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { problemCode, throwProblem, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { WebhookSubscriptionDrawer } from "./webhookdrawer";
import "./webhooks.css";

import { DeliveriesPanel } from "./webhooks.deliveries";
import { RotateSecretAction, SecretRevealModal } from "./webhooks.secret";

// Settings → Integrations (B-E10.14): the subscription list for outbound
// webhooks. The list wire (WebhookSubscription) carries no per-item delivery
// health — that lives on the separate deliveries sub-resource, out of this
// card's scope — so the health line here renders only what the list itself
// is honest about: state, the subscribed event set, and last-updated. A
// deployment with no signing key answers 503 webhooks_not_configured; that
// is a deliberate, documented feature-off state, never an error.

export { webhookStatusBadge } from "./webhooks.deliveries";

export type WebhookSubscription = components["schemas"]["WebhookSubscription"];

export type WebhookDelivery = components["schemas"]["WebhookDelivery"];

export type WebhookDeliveryStatus = WebhookDelivery["status"];

export type WebhookSubscriptionCreated =
  components["schemas"]["WebhookSubscriptionCreated"];

export type WebhookDeliveryListResponse =
  components["schemas"]["WebhookDeliveryListResponse"];

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

function subscriptionStateTone(
  state: WebhookSubscription["state"],
): "success" | "warning" {
  return state === "active" ? "success" : "warning";
}

function NotConfiguredState() {
  const t = useT();
  return <EmptyState>{t("webhooks.notConfigured")}</EmptyState>;
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
                      requires={{
                        object: "webhook_subscription",
                        action: "delete",
                      }}
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
