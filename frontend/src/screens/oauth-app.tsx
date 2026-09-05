import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
import { Button, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

/**
 * A connector OAuth app a mailbox connection is made through — Google's, or
 * Microsoft's.
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

export type Vendor = "google" | "microsoft";

// vendorCopy is each vendor's own wording, as literal message keys.
//
// A record of literals rather than a template built from the provider name: the
// translator's key type is a closed union, so a computed key types as a plain
// string and a missing translation would reach a reader as a raw key instead of
// failing the build.
export const vendorCopy = {
  google: {
    title: "oauthApp.google.title",
    sub: "oauthApp.google.sub",
    absent: "oauthApp.google.absent",
    redirectSub: "oauthApp.google.redirectSub",
    clientIdPlaceholder: "oauthApp.google.clientIdPlaceholder",
    removeConfirmTitle: "oauthApp.google.removeConfirmTitle",
    removeConfirmBody: "oauthApp.google.removeConfirmBody",
  },
  microsoft: {
    title: "oauthApp.microsoft.title",
    sub: "oauthApp.microsoft.sub",
    absent: "oauthApp.microsoft.absent",
    redirectSub: "oauthApp.microsoft.redirectSub",
    clientIdPlaceholder: "oauthApp.microsoft.clientIdPlaceholder",
    removeConfirmTitle: "oauthApp.microsoft.removeConfirmTitle",
    removeConfirmBody: "oauthApp.microsoft.removeConfirmBody",
  },
} as const;

// The query key carries the vendor, so storing one app does not blank the
// other's card — one key for both would make every write invalidate a view it
// says nothing about.
function appQueryKey(provider: Vendor) {
  return ["installation-oauth-app", provider];
}

export function useOAuthApp(provider: Vendor) {
  return useQuery({
    queryKey: appQueryKey(provider),
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/installation/oauth-apps/{provider}",
        { params: { path: { provider } } },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function useSetOAuthApp(provider: Vendor) {
  const queryClient = useQueryClient();
  return useMutation({
    // Collected the moment nothing observes it, because what this mutation's
    // `variables` hold is a credential rather than a form field. The caller
    // resets it once settled for the same reason.
    gcTime: 0,
    mutationFn: async (vars: {
      clientId: string;
      clientSecret: string;
      tenant: string;
    }) => {
      const { error } = await api.PUT("/installation/oauth-apps/{provider}", {
        params: { path: { provider } },
        body: {
          client_id: vars.clientId,
          client_secret: vars.clientSecret,
          // Omitted rather than sent empty: the server refuses a tenant on a
          // vendor that has no directories, and an empty string is a value.
          ...(vars.tenant === "" ? {} : { tenant: vars.tenant }),
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      // All three: the card's own view, the setup report (which names this
      // step as outstanding whether or not it blocks anything), and the
      // connector roster, whose per-provider availability is decided by
      // whether this app exists.
      await queryClient.invalidateQueries({ queryKey: appQueryKey(provider) });
      await queryClient.invalidateQueries({ queryKey: ["installation-setup"] });
      await queryClient.invalidateQueries({ queryKey: ["connectors"] });
    },
  });
}

function useRemoveOAuthApp(provider: Vendor) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE(
        "/installation/oauth-apps/{provider}",
        { params: { path: { provider } } },
      );
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: appQueryKey(provider) });
      await queryClient.invalidateQueries({ queryKey: ["installation-setup"] });
    },
  });
}

// purposeLabel names one callback's flow. A lookup rather than a computed
// message key, because the catalog is a closed union: a template key would type
// as any string and a purpose the contract adds later would reach a reader as a
// raw key rather than as words.
//
// Every purpose the contract declares has an arm. A missing one renders the raw
// enum beside two translated rows, which is what a reader takes for a bug in the
// list rather than in this switch.
function purposeLabel(purpose: string, t: ReturnType<typeof useT>): string {
  switch (purpose) {
    case "sign_in":
      return t("oauthApp.redirect.sign_in");
    case "mailbox_connect":
      return t("oauthApp.redirect.mailbox_connect");
    case "calendar_connect":
      return t("oauthApp.redirect.calendar_connect");
    default:
      return purpose;
  }
}

// RedirectUris lists the callback URLs an operator must register on their OAuth
// client, one per purpose this deployment actually serves.
//
// The URLs come from the response and are never built here. They have one job —
// to be byte-identical to what the vendor receives — and a second spelling in the
// client is exactly how the two come apart. The backend derives them from the
// functions that send them, held by a fitness test in both directions.
//
// An empty list renders nothing rather than an empty heading: a deployment that
// serves none of a vendor's flows has nothing to register, and a bare heading
// would read as a list that failed to load.
export function RedirectUris({
  uris,
  sub,
}: Readonly<{
  uris: readonly { purpose: string; url: string }[] | undefined;
  sub: string;
}>) {
  const t = useT();
  const [copied, setCopied] = useState("");
  // Absent and empty are the same answer here — nothing to register — and the
  // field is contract-required, but a body that lost one hands over `undefined`
  // anyway. This card shares a screen with the installation's own settings, so
  // reading a length off nothing would take that whole page down over a list
  // the reader could not have acted on.
  if (!uris || uris.length === 0) {
    return null;
  }
  return (
    <div className="stack-sm">
      <p className="t-label">{t("oauthApp.redirectTitle")}</p>
      <p className="t-caption">{sub}</p>
      <SettingList>
        {uris.map((uri) => {
          const purpose = purposeLabel(uri.purpose, t);
          return (
            <SettingRow
              key={uri.purpose}
              label={purpose}
              value={<code className="t-mono">{uri.url}</code>}
              control={
                <Button
                  small
                  onClick={() => {
                    // The object itself is guarded, not just the promise: the
                    // Clipboard API is absent outside a secure context, so on a
                    // plain-HTTP deployment this THROWS rather than rejecting and
                    // the rejection handler never runs. A denied permission is
                    // the rejecting case. Either way the URL stays on screen to
                    // copy by hand, so the honest failure is to stop claiming it
                    // was copied.
                    if (!navigator.clipboard) {
                      return;
                    }
                    navigator.clipboard.writeText(uri.url).then(
                      () => setCopied(uri.purpose),
                      () => setCopied(""),
                    );
                  }}
                >
                  {copied === uri.purpose
                    ? t("oauthApp.redirectCopied")
                    : t("oauthApp.redirectCopy", { purpose })}
                </Button>
              }
            />
          );
        })}
      </SettingList>
    </div>
  );
}

export function OAuthAppCard({ provider }: Readonly<{ provider: Vendor }>) {
  const t = useT();
  const copy = vendorCopy[provider];
  const canManage = useCanWrite("capture_settings", "update");
  const app = useOAuthApp(provider);
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
  // would send no tenant and silently widen it to every organization, which is
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

  return (
    <Panel title={t(copy.title)}>
      <PanelBody>
        <p className="t-body">{t(copy.sub)}</p>
        <QueryGate query={app} pendingLabel={t(copy.title)}>
          {(status) => (
            <>
              {failure && (
                <Callout tone="danger">{problemMessageOf(failure, t)}</Callout>
              )}
              {/* Three states, because there are three answers. `configured`
                  alone could not tell "nothing anywhere" from "the deployment
                  supplies one", and the card said the former in both — telling
                  an operator mail could not be connected on installations
                  where it demonstrably could. */}
              <p className="t-body">
                {status.source === "stored" &&
                  t("oauthApp.configured", { clientId: status.client_id })}
                {status.source === "environment" &&
                  t("oauthApp.fromEnvironment", {
                    clientId: status.client_id,
                  })}
                {status.source === "none" && t(copy.absent)}
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
              <RedirectUris
                uris={status.redirect_uris}
                sub={t(copy.redirectSub)}
              />
              <Field
                label={t("oauthApp.clientId")}
                hint={
                  status.source === "stored"
                    ? t("oauthApp.replaceHint")
                    : undefined
                }
              >
                {(control) => (
                  <TextInput
                    {...control}
                    ref={clientIdRef}
                    value={clientId}
                    autoComplete="off"
                    disabled={!canManage || busy}
                    placeholder={t(copy.clientIdPlaceholder)}
                    onChange={(e) => setClientId(e.target.value)}
                  />
                )}
              </Field>
              <Field label={t("oauthApp.clientSecret")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="password"
                    autoComplete="off"
                    value={clientSecret}
                    disabled={!canManage || busy}
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
                      // The stored directory until somebody edits it, so a
                      // rotation carries the pinning forward. Emptying the
                      // field is then a deliberate act, which is what widening
                      // an app to every organization ought to be.
                      value={tenant ?? status.tenant ?? ""}
                      autoComplete="off"
                      disabled={!canManage || busy}
                      placeholder={t("oauthApp.tenantPlaceholder")}
                      onChange={(e) => setTenant(e.target.value)}
                    />
                  )}
                </Field>
              )}
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
                        tenant: (tenant ?? status.tenant ?? "").trim(),
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
                  {status.source === "stored"
                    ? t("oauthApp.replace")
                    : t("oauthApp.store")}
                </Button>
                {status.source === "stored" && (
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
              <ConfirmModal
                open={confirming}
                onClose={() => setConfirming(false)}
                title={t(copy.removeConfirmTitle)}
                confirmLabel={t("oauthApp.remove")}
                confirmVariant="danger"
                pending={remove.isPending}
                // A refused DELETE leaves this dialog OPEN, so the reason has
                // to be inside it: the background Callout is behind a modal the
                // reader cannot see past, which reads as a button that did
                // nothing.
                error={remove.error ? problemMessageOf(remove.error, t) : null}
                returnFocusTo={() => clientIdRef.current}
                onConfirm={() => {
                  save.reset();
                  remove.mutate(undefined, {
                    onSuccess: () => {
                      setConfirming(false);
                      // AND THE DRAFT GOES WITH IT. An operator who typed
                      // replacement credentials and then removed the app
                      // instead was left with both fields populated and Store
                      // app ready to press — offering to store a secret they
                      // had just decided to be rid of, against an app that no
                      // longer exists.
                      setClientSecret("");
                      setClientId("");
                      setTenant(undefined);
                    },
                  });
                }}
              >
                {t(copy.removeConfirmBody)}
              </ConfirmModal>
            </>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}
