// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useEffect, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  Card,
  Checkbox,
  EmptyState,
  SegmentedControl,
} from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import "./oauthconsent.css";

// The human hands an agent their own authority here — the one screen where
// that decision is made, and the only one: the api serves no HTML, so there is
// no other surface a consent decision can be taken on. GET /oauth/authorize
// arms a single-use nonce, sets an HttpOnly Path=/oauth/authorize cookie
// carrying its counterpart, and 302s to `/#/oauth-consent?…&consent=<nonce>`.
// The nonce is deliberately absent from the consent-request endpoint's
// response (the cookie never reaches it), so it is read out of the redirect
// fragment instead and POSTed back — the POST proves possession of both
// halves. Every one of these params must ride the POST unchanged, because
// the server re-validates the whole request against what the GET armed.
//
// A refused POST comes back here with `error=<marker>`, and whether the nonce
// comes back with it is the server's statement about what is left to do: a
// terminal one hands back the request alone, and there is no recoverable
// refusal that carries a nonce back any more — every scope the client could
// ask for is offered here, ticked, so there is nothing left to retry with a
// narrower choice. Hence the rule this screen keeps: a submittable form is
// rendered only where a nonce is actually held.
const AUTHORIZE_PARAMS = [
  "response_type",
  "client_id",
  "redirect_uri",
  "scope",
  "code_challenge",
  "code_challenge_method",
  "resource",
  "state",
] as const;

function fragmentParams(): URLSearchParams {
  return new URLSearchParams(globalThis.location.hash.split("?")[1] ?? "");
}

// The un-consented authorize query — every fragment param EXCEPT the nonce.
// Replaying the nonce would defeat the point of it being single-use and
// cookie-bound, so a fresh re-entry into /oauth/authorize is the only way to
// arm one.
function reauthorizeUrl(params: URLSearchParams): string {
  const carried = new URLSearchParams();
  for (const key of AUTHORIZE_PARAMS) {
    const value = params.get(key);
    if (value !== null) {
      carried.set(key, value);
    }
  }
  return `/oauth/authorize?${carried.toString()}`;
}

// The hidden fields both the Authorize and the Cancel form share: the whole
// authorize request plus the nonce, carried through untouched.
function HiddenAuthorizeFields({
  params,
  consent,
}: Readonly<{ params: URLSearchParams; consent: string }>) {
  return (
    <>
      {AUTHORIZE_PARAMS.map((key) => {
        const value = params.get(key);
        return value === null ? null : (
          <input key={key} type="hidden" name={key} value={value} />
        );
      })}
      <input type="hidden" name="consent" value={consent} />
    </>
  );
}

type ConsentRequest = components["schemas"]["ConsentRequest"];
type Scope = ConsentRequest["scopes"][number];

// A refusal with no forward action of its own — the recovery lives back at
// the client, not on this screen — so it gets the one thing every other
// state here already has: a way out of a rail-less screen that would
// otherwise be a dead end.
//
// It takes message KEYS rather than a server-supplied client name, because it
// has to render without the consent-request fetch: the likeliest cause of
// invalid_request is a client that went unknown, disabled or deleted, which is
// exactly what makes that fetch 404. Copy that named the client would either
// print an empty name or force this state behind data it cannot have.
function ConsentErrorCard({
  titleKey,
  bodyKey,
  children,
}: Readonly<{
  titleKey: MessageKey;
  bodyKey: MessageKey;
  children?: ReactNode;
}>) {
  const t = useT();
  return (
    <Card>
      <Heading size="xlarge">{t(titleKey)}</Heading>
      <p>{t(bodyKey)}</p>
      {children}
      <Button variant="ghost" onClick={() => navigate({ screen: "home" })}>
        {t("consent.backToApp")}
      </Button>
    </Card>
  );
}

// Toggling one scope in or out of the granted set. A plain ternary expression
// statement reads as two ideas on one line (which branch, and the mutation
// itself); this keeps the toggle to the one idea it is.
function toggled(current: ReadonlySet<Scope>, scope: Scope): Set<Scope> {
  const next = new Set(current);
  if (next.has(scope)) {
    next.delete(scope);
  } else {
    next.add(scope);
  }
  return next;
}

// The two common grants, offered as one choice above the boxes. "Read and
// draft" lets an assistant look and prepare without changing or sending
// anything; "Full assistant" is every scope offered. Any other set is custom.
const PRESETS = ["readDraft", "full", "custom"] as const;
type Preset = (typeof PRESETS)[number];
const READ_AND_DRAFT: readonly Scope[] = ["read", "draft"];

function presetOf(
  granted: ReadonlySet<Scope>,
  offered: readonly Scope[],
): Preset {
  const readDraft = offered.filter((scope) => READ_AND_DRAFT.includes(scope));
  const exactly = (set: readonly Scope[]) =>
    set.length === granted.size && set.every((scope) => granted.has(scope));
  if (exactly(offered)) return "full";
  if (exactly(readDraft)) return "readDraft";
  return "custom";
}

function presetScopes(
  preset: Preset,
  offered: readonly Scope[],
  current: ReadonlySet<Scope>,
): ReadonlySet<Scope> {
  switch (preset) {
    case "full":
      return new Set(offered);
    case "readDraft":
      return new Set(offered.filter((scope) => READ_AND_DRAFT.includes(scope)));
    case "custom":
      return current;
  }
}

// The ordinary path: every scope the client could ask for, ticked by default.
// `consent` is a real nonce here, never "": OAuthConsent renders this only past
// its own no-nonce guard.
function ConsentSelector({
  data,
  params,
  consent,
}: Readonly<{
  data: ConsentRequest;
  params: URLSearchParams;
  consent: string;
}>) {
  const t = useT();
  // Ticked by default: a connection that can only read is not what someone
  // connecting an assistant is asking for, and the first thing they try would
  // fail in a way that reads as the product being broken.
  const [granted, setGranted] = useState<ReadonlySet<Scope>>(
    () => new Set(data.scopes),
  );
  const scopeList = data.scopes.filter((scope) => granted.has(scope)).join(" ");

  return (
    <Card>
      <Heading size="xlarge">{t("consent.title")}</Heading>
      <p>{t("consent.asks", { client: data.client_name })}</p>
      <RedirectDisclosure redirectURI={params.get("redirect_uri") ?? ""} />
      <SegmentedControl
        options={PRESETS}
        value={presetOf(granted, data.scopes)}
        onChange={(preset) =>
          setGranted((current) => presetScopes(preset, data.scopes, current))
        }
        labels={{
          readDraft: t("consent.preset.readDraft"),
          full: t("consent.preset.full"),
          custom: t("consent.preset.custom"),
        }}
        label={t("consent.preset.label")}
      />
      <div className="oauthconsent-scopes">
        {data.scopes.map((scope) => (
          <Checkbox
            key={scope}
            checked={granted.has(scope)}
            onChange={() => setGranted((current) => toggled(current, scope))}
            label={
              <>
                <strong>{t(`passport.scope.${scope}`)}</strong>{" "}
                <span className="t-caption">
                  {t(`consent.scopeNote.${scope}`)}
                </span>
              </>
            }
          />
        ))}
      </div>
      <p className="t-caption">{t("consent.ceiling")}</p>
      {data.offline && <p>{t("consent.offline")}</p>}
      <div className="oauthconsent-verbs">
        <form method="post" action="/oauth/authorize">
          <HiddenAuthorizeFields params={params} consent={consent} />
          <input type="hidden" name="scopes" value={scopeList} />
          <Button type="submit" variant="primary" disabled={granted.size === 0}>
            {t("consent.approve")}
          </Button>
        </form>
        <form method="post" action="/oauth/authorize">
          <HiddenAuthorizeFields params={params} consent={consent} />
          <input type="hidden" name="deny" value="1" />
          <Button type="submit">{t("consent.deny")}</Button>
        </form>
        {granted.size === 0 && <p>{t("consent.pickOne")}</p>}
      </div>
    </Card>
  );
}

export function OAuthConsent() {
  const t = useT();
  // Read once per mount: the fragment is the SPA's own address bar for this
  // screen, not a value that changes while it's open.
  const [params] = useState(fragmentParams);
  const clientId = params.get("client_id") ?? "";
  const scope = params.get("scope") ?? "";
  const consent = params.get("consent") ?? "";
  const errorCode = params.get("error");
  const me = useMe();

  const query = useQuery({
    queryKey: ["oauth-consent-request", clientId, scope],
    // Only a render that can actually offer the human a decision needs the
    // scope list, and that is exactly the render holding a nonce. The states
    // below without one — the re-entry detour and the terminal refusals —
    // return before this query is ever read.
    enabled: Boolean(consent) || errorCode === "method_off",
    queryFn: async () => {
      return unwrap(
        await api.GET("/oauth/consent-request", {
          params: { query: { client_id: clientId, scope } },
        }),
      );
    },
  });

  // The not-signed-in case 302s here with no nonce at all (rather than a
  // bare 401), so App's own auth gate can render the login screen in place
  // with the hash preserved. OAuthConsent only ever mounts once that gate
  // has resolved PAST pending/error (App.tsx's AuthedApp only renders
  // ScreenView then) — so `me.data` here is not a guess about whether a
  // session exists, it is the same signal the gate already used to decide
  // this screen renders at all. Once it does, re-enter /oauth/authorize
  // with the same params (still nonce-free — a fresh one is only ever
  // minted server-side) to obtain the nonce this render is missing.
  //
  // Loop freedom: this effect fires only while BOTH `consent` and
  // `errorCode` are absent. The server's own contract (oauth_consentscreen.go)
  // means a request replayed with a session already accepted here always
  // resolves to one or the other on the next redirect — never back to the
  // bare no-nonce shape this effect reacts to — so it cannot re-arm itself a
  // second time for the same visit. A session that never resolves (the
  // structurally-unreachable case where this component mounts without one)
  // leaves this screen showing "reconnecting" forever rather than looping —
  // a dead end is the safe failure, not a loop.
  useEffect(() => {
    if (!consent && !errorCode && me.data) {
      globalThis.location.assign(reauthorizeUrl(params));
    }
  }, [consent, errorCode, me.data, params]);

  if (!consent && !errorCode) {
    return (
      <div className="wrap narrow">
        <EmptyState>{t("consent.reentering")}</EmptyState>
      </div>
    );
  }

  // The refusals nothing on this screen can act on, rendered BEFORE the
  // consent-request fetch because neither needs it — and invalid_request most
  // often cannot have it: its likeliest cause is a client that went unknown,
  // disabled or deleted, which makes that same fetch 404. Behind the gate, the
  // one sentence that tells the human what to do became "couldn't load this
  // view" with a Retry button that retries the wrong thing.
  if (errorCode === "invalid_request") {
    return (
      <div className="wrap narrow">
        <ConsentErrorCard
          titleKey="consent.invalidTitle"
          bodyKey="consent.invalidBody"
        />
      </div>
    );
  }
  // A way of connecting the admin turned off: only the admin can change that.
  // The client is still live, so its name can be read; the screen renders
  // without it while the read is out or when it fails.
  if (errorCode === "method_off") {
    return (
      <div className="wrap narrow">
        <ConsentErrorCard
          titleKey="consent.methodOffTitle"
          bodyKey="consent.methodOffBody"
        >
          <MethodOffFacts
            clientName={query.data?.client_name}
            redirectURI={params.get("redirect_uri") ?? ""}
          />
        </ConsentErrorCard>
      </div>
    );
  }
  // stale_consent says outright that the request is spent — and ANY arrival
  // without a nonce is the same fact, whatever marker it carries: the POST
  // requires cookie and body to agree, so a selector rendered here could only
  // offer a submission the server must refuse.
  if (errorCode === "stale_consent" || !consent) {
    return (
      <div className="wrap narrow">
        <ConsentErrorCard
          titleKey="consent.staleTitle"
          bodyKey="consent.staleBody"
        />
      </div>
    );
  }

  return (
    <div className="wrap narrow">
      <QueryGate query={query} pendingLabel={t("consent.title")}>
        {(data) => (
          <ConsentSelector data={data} params={params} consent={consent} />
        )}
      </QueryGate>
    </div>
  );
}

// Which tool was refused and where it would have returned, so the human can
// tell their admin which one to let in.
function MethodOffFacts({
  clientName,
  redirectURI,
}: Readonly<{ clientName: string | undefined; redirectURI: string }>) {
  const t = useT();
  const host = URL.canParse(redirectURI) ? new URL(redirectURI).host : "";
  return (
    <div className="oauthconsent-facts t-caption">
      {clientName ? (
        <span>{t("consent.methodOffTool", { client: clientName })}</span>
      ) : null}
      {host ? <span>{t("consent.methodOffReturn", { host })}</span> : null}
    </div>
  );
}

// The redirect the authorization code will be sent to, named by HOST.
//
// The 2026-07-28 profile makes this a MUST for a reason a human can act on: a
// client id is a URL and a client name is whatever that URL's document says it
// is, so the destination is the one fact about this connection that nobody but
// the human can judge. A loopback destination carries an extra line, because a
// metadata document cannot prove WHO is listening on a port on this machine —
// the profile's own words, and the one case where "the name looks right" is not
// enough.
function RedirectDisclosure({ redirectURI }: { redirectURI: string }) {
  const t = useT();
  let host: string;
  let loopback: boolean;
  try {
    const parsed = new URL(redirectURI);
    host = parsed.host;
    loopback =
      parsed.hostname === "localhost" ||
      parsed.hostname === "127.0.0.1" ||
      parsed.hostname === "[::1]";
  } catch {
    // An unparseable redirect never reaches a code — the server refuses it
    // before this screen is rendered — so there is nothing honest to name.
    return null;
  }
  return (
    <p className="t-caption">
      {t("consent.redirectsTo", { host })}
      {loopback ? ` ${t("consent.redirectsToLoopback")}` : ""}
    </p>
  );
}
