import { useQuery } from "@tanstack/react-query";
import { useId, useRef, useState } from "react";
import { api } from "../../api/client";
import { RedirectUriGroup } from "../../components/oauth-redirects";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { describedByIds } from "../../design-system/describedby";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, QueryStates } from "../../design-system/querystates";
import { lockFor } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import {
  APP_LIST_KEY,
  appKey,
  useOAuthApp,
  useRemoveOAuthApp,
  useSetOAuthApp,
  type Vendor,
  vendorCopy,
} from "../../lib/oauth-app";
import { problemMessageOf, throwProblem } from "../../lib/problem";

/**
 * An OAuth app this installation holds: Google's or Microsoft's, which mailbox
 * connections and sign-in go through, or one an extension unit declares.
 *
 * ONE card, rendered once per vendor. The two ask for the same three things and
 * differ only in the words around them, so a second component would be a second
 * answer to how long a client SECRET lives in a mutation's `variables` — and the
 * two would drift on exactly that.
 *
 * The secret is never read back. `GET` answers whether one is stored and the
 * client id, which is not a secret — it travels in every authorization redirect,
 * and an operator needs to see WHICH app their installation uses to check it
 * against the vendor's console.
 */

/**
 * Which app a card is about: a core vendor by its key, or a unit's app by its
 * provider and its title.
 */
export type OAuthAppIdentity =
  | Readonly<{ provider: Vendor; title?: undefined }>
  | Readonly<{ provider: string; title: string }>;

type CardCopy = Readonly<{
  title: string;
  sub: string;
  absent: string;
  redirectSub: string;
  clientIdPlaceholder?: string;
  removeConfirmTitle: string;
  removeConfirmBody: string;
}>;

// A unit's app is worded around its title, since the core knows nothing else
// about the vendor.
function cardCopy(app: OAuthAppIdentity, t: ReturnType<typeof useT>): CardCopy {
  if (app.title === undefined) {
    const copy = vendorCopy[app.provider];
    return {
      title: t(copy.title),
      sub: t(copy.sub),
      absent: t(copy.absent),
      redirectSub: t(copy.redirectSub),
      clientIdPlaceholder: t(copy.clientIdPlaceholder),
      removeConfirmTitle: t(copy.removeConfirmTitle),
      removeConfirmBody: t(copy.removeConfirmBody),
    };
  }
  const title = { title: app.title };
  return {
    title: app.title,
    sub: t("oauthApp.unit.sub", title),
    absent: t("oauthApp.unit.absent", title),
    redirectSub: t("oauthApp.unit.redirectSub"),
    removeConfirmTitle: t("oauthApp.unit.removeConfirmTitle", title),
    removeConfirmBody: t("oauthApp.unit.removeConfirmBody", title),
  };
}

// The deployment's app is the one every connect uses, so it locks this form: a
// stored app would be saved and never read. `lockReasonId` is set only while the
// reason is drawn.
function useAppLock(
  loaded:
    | { locks?: Parameters<typeof lockFor>[0]; configured: boolean }
    | undefined,
  lockKey: string | undefined,
) {
  const t = useT();
  const lockId = useId();
  // A unit's app has no deployment setting, so no lock names it.
  const lock = lockFor(loaded?.locks, lockKey ?? "", t);
  return {
    lock,
    lockId,
    lockReasonId: lock.disabled ? lockId : undefined,
    stored: loaded?.configured === true && !lock.disabled,
  };
}

/**
 * A card per OAuth app the composed extension units declare, in the list's
 * order. Nothing renders on an installation whose units declare none, nor
 * while the list loads, since most have none to wait for.
 */
export function UnitOAuthAppCards() {
  const t = useT();
  const apps = useQuery({
    queryKey: APP_LIST_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/installation/oauth-apps",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
  });
  if (apps.isError) {
    const title = t("oauthApp.unit.listTitle");
    return (
      <Panel title={title}>
        <PanelBody>
          <QueryStates query={apps} pendingLabel={title}>
            {null}
          </QueryStates>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <>
      {(apps.data ?? []).map((app) =>
        app.title === undefined ? null : (
          <OAuthAppCard
            key={app.provider}
            provider={app.provider}
            title={app.title}
          />
        ),
      )}
    </>
  );
}

export function OAuthAppCard(app: OAuthAppIdentity) {
  const { provider } = app;
  const t = useT();
  const copy = cardCopy(app, t);
  const canManage = useCanWrite("capture_settings", "update");
  const appQuery = useOAuthApp(provider);
  const save = useSetOAuthApp(provider);
  const remove = useRemoveOAuthApp(provider);
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  // Microsoft only: an Entra app may be pinned to one directory, and Google has
  // no such concept — a field that silently does nothing is worse than an absent
  // one, because an operator who fills it in believes they narrowed something.
  //
  // `undefined` until the app has loaded, which is what separates "not typed in
  // yet" from "deliberately cleared": rotating a pinned app with this blank
  // would send no tenant and silently widen it to every company, which is
  // the one change on this card nobody would see happen.
  const [tenant, setTenant] = useState<string | undefined>(undefined);
  const [confirming, setConfirming] = useState(false);
  // Focus has to land somewhere that still exists. The button that opened the
  // dialog is the natural target and the one that disappears: a successful
  // removal invalidates the query, the app reads as absent, and the Remove
  // button is gone by the time focus is restored to it.
  const clientIdRef = useRef<HTMLInputElement>(null);

  const busy = save.isPending || remove.isPending;
  const ready = clientId.trim() !== "" && clientSecret.trim() !== "";
  const failure = save.error ?? remove.error;
  // A failed refetch keeps the old data while the gate above offers Retry.
  const loaded = appQuery.isError ? undefined : appQuery.data;
  const { lock, lockId, lockReasonId, stored } = useAppLock(
    loaded,
    app.title === undefined ? appKey[app.provider] : undefined,
  );
  const replaceHint = stored ? t("oauthApp.replaceHint") : undefined;

  return (
    <Panel title={copy.title}>
      <PanelBody>
        <PanelIntro>{copy.sub}</PanelIntro>
        <QueryGate query={appQuery} pendingLabel={copy.title}>
          {(status) => (
            <>
              {failure && (
                <Callout
                  kind="outcome"
                  tone="danger"
                  title={t("oauthApp.writeFailed")}
                >
                  {problemMessageOf(failure, t)}
                </Callout>
              )}
              {/* `configured` is true for the deployment's app too; its lock,
                  under the fields, says where that one is set. */}
              <p className="t-body">
                {status.configured
                  ? t("oauthApp.configured", { clientId: status.client_id })
                  : copy.absent}
              </p>
              {/* Whatever the app's source: a directory pin is a fact about
                  who can sign in, and an operator debugging a refused login
                  needs it whether the app came from the environment or from
                  this screen. */}
              {status.tenant && (
                <p className="t-caption">
                  {t("oauthApp.pinnedToDirectory", { tenant: status.tenant })}
                </p>
              )}
            </>
          )}
        </QueryGate>
      </PanelBody>
      {loaded && (
        <PanelBody>
          <RedirectUriGroup
            open={!loaded.configured}
            uris={loaded.redirect_uris}
            sub={copy.redirectSub}
          />
          <div className="form-stack">
            {lock.disabled && (
              <p id={lockId} className="t-caption">
                {lock.reason}
              </p>
            )}
            <Field label={t("oauthApp.clientId")} hint={replaceHint}>
              {(control) => (
                <TextInput
                  {...control}
                  aria-describedby={describedByIds(
                    control["aria-describedby"],
                    lockReasonId,
                  )}
                  ref={clientIdRef}
                  value={clientId}
                  autoComplete="off"
                  disabled={!canManage || busy || lock.disabled}
                  placeholder={copy.clientIdPlaceholder}
                  onChange={(e) => setClientId(e.target.value)}
                />
              )}
            </Field>
            <Field label={t("oauthApp.clientSecret")}>
              {(control) => (
                <TextInput
                  {...control}
                  aria-describedby={describedByIds(
                    control["aria-describedby"],
                    lockReasonId,
                  )}
                  type="password"
                  autoComplete="off"
                  value={clientSecret}
                  disabled={!canManage || busy || lock.disabled}
                  onChange={(e) => setClientSecret(e.target.value)}
                />
              )}
            </Field>
            {provider === "microsoft" && (
              <Field
                label={t("oauthApp.tenant")}
                hint={t("oauthApp.tenantHint")}
              >
                {(control) => (
                  <TextInput
                    {...control}
                    aria-describedby={describedByIds(
                      control["aria-describedby"],
                      lockReasonId,
                    )}
                    // The stored directory until somebody edits it, so a
                    // rotation carries the pinning forward. Widening an app
                    // to every company then takes emptying the field.
                    value={tenant ?? loaded.tenant ?? ""}
                    autoComplete="off"
                    disabled={!canManage || busy || lock.disabled}
                    placeholder={t("oauthApp.tenantPlaceholder")}
                    onChange={(e) => setTenant(e.target.value)}
                  />
                )}
              </Field>
            )}
            {/* A locked app refuses every value, its own included, so no
                verb is offered at all. */}
            {!lock.disabled && (
              <div className="form-actions">
                <Button
                  variant="primary"
                  pending={save.isPending}
                  disabled={!canManage || remove.isPending || !ready}
                  onClick={() => {
                    remove.reset();
                    save.mutate(
                      {
                        clientId: clientId.trim(),
                        clientSecret: clientSecret.trim(),
                        tenant: (tenant ?? loaded.tenant ?? "").trim(),
                      },
                      {
                        onSuccess: () => {
                          // The field is the only copy this app holds, and it
                          // has done its job.
                          setClientSecret("");
                          setClientId("");
                          setTenant(undefined);
                          save.reset();
                        },
                      },
                    );
                  }}
                >
                  {stored ? t("oauthApp.replace") : t("oauthApp.store")}
                </Button>
                {stored && (
                  <Button
                    variant="danger"
                    pending={remove.isPending}
                    disabled={!canManage || save.isPending}
                    onClick={() => setConfirming(true)}
                  >
                    {t("oauthApp.remove")}
                  </Button>
                )}
              </div>
            )}
            <ConfirmModal
              open={confirming}
              onClose={() => setConfirming(false)}
              title={copy.removeConfirmTitle}
              confirmLabel={t("oauthApp.remove")}
              confirmVariant="danger"
              pending={remove.isPending}
              // A refused DELETE leaves this dialog open, so the reason goes
              // inside it. The background Callout sits behind the modal.
              error={remove.error ? problemMessageOf(remove.error, t) : null}
              returnFocusTo={() => clientIdRef.current}
              onConfirm={() => {
                save.reset();
                remove.mutate(undefined, {
                  onSuccess: () => {
                    setConfirming(false);
                    // The draft goes too: a removed app must not leave a
                    // secret ready to store against an app that is gone.
                    setClientSecret("");
                    setClientId("");
                    setTenant(undefined);
                  },
                });
              }}
            >
              {copy.removeConfirmBody}
            </ConfirmModal>
          </div>
        </PanelBody>
      )}
    </Panel>
  );
}
