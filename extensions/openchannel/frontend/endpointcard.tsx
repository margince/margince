import { api, throwProblem } from "@margince/frontend/api";
import {
  formatNumber,
  useCanWrite,
  useLocale,
  useT,
} from "@margince/frontend/app";
import {
  Badge,
  Button,
  Callout,
  type Fact,
  FactList,
  Field,
  SectionHeader,
  TextInput,
} from "@margince/frontend/design-system";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  ENDPOINT_KEY,
  ENDPOINT_OBJECT,
  type Endpoint,
  INBOUND_KEY,
  OUTBOUND_KEY,
} from "./contract";
import { Recipe } from "./recipe";

// The member's own endpoint: whether one is open, what state it is in, and the
// four things they can do to it.
//
// EVERY CONTROL HERE IS ABSENT RATHER THAN DISABLED for a seat that may only
// read. A disabled control still says "this is yours, later"; an absent one is
// the truth, and the alternative — a live control leading to a 403 — is worse
// than either.

/**
 * The half of a mint's answer this screen reads.
 *
 * The endpoint row travels back with it and is deliberately not taken from
 * here: the screen re-reads it through the query the mint invalidates, so
 * there is one answer to "what is my endpoint" rather than two that can
 * disagree.
 */
type Minted = { signing_secret: string };

/**
 * The three reads a write can invalidate.
 *
 * Opening an endpoint changes what the listings answer, and so does pausing
 * one, so the invalidation covers all three rather than only the row the
 * mutation returned.
 */
function useRefreshEverything() {
  const queryClient = useQueryClient();
  return async () => {
    for (const key of [ENDPOINT_KEY, INBOUND_KEY, OUTBOUND_KEY]) {
      await queryClient.invalidateQueries({ queryKey: key });
    }
  };
}

export function EndpointCard({
  endpoint,
}: Readonly<{ endpoint: Endpoint | null | undefined }>) {
  const t = useT();
  const canOpen = useCanWrite(ENDPOINT_OBJECT, "create");
  const canChange = useCanWrite(ENDPOINT_OBJECT, "update");
  return (
    <>
      <SectionHeader
        title={t("extOpenchannel.endpoint.title")}
        sub={t("extOpenchannel.endpoint.sub")}
      />
      {endpoint ? (
        <OpenedEndpoint endpoint={endpoint} canChange={canChange} />
      ) : (
        <AbsentEndpoint canOpen={canOpen} />
      )}
    </>
  );
}

/**
 * Not having opened an endpoint is the ordinary first state of this screen,
 * not a failure — so it is a fact and a verb rather than an error card.
 */
function AbsentEndpoint({ canOpen }: Readonly<{ canOpen: boolean }>) {
  const t = useT();
  const refresh = useRefreshEverything();
  const open = useMutation({
    mutationFn: async () => {
      const { error, response } = await api.PUT("/ext/openchannel/endpoint", {
        body: {},
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
    },
    // onSettled rather than onSuccess: an answer lost on the way back leaves
    // the endpoint open while this client sees a failure, so the screen
    // re-reads instead of asserting a rollback it cannot know about.
    onSettled: refresh,
  });
  return (
    <>
      <p>
        <Badge tone="warn">{t("extOpenchannel.endpoint.absent")}</Badge>
      </p>
      {canOpen ? (
        <div className="card-actions">
          <Button disabled={open.isPending} onClick={() => open.mutate()}>
            {t("extOpenchannel.endpoint.open")}
          </Button>
        </div>
      ) : null}
      {open.isError ? (
        <p role="alert">{t("extOpenchannel.endpoint.openFailed")}</p>
      ) : null}
    </>
  );
}

function OpenedEndpoint({
  endpoint,
  canChange,
}: Readonly<{ endpoint: Endpoint; canChange: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const facts: Fact[] = [
    {
      key: "received",
      term: t("extOpenchannel.endpoint.received"),
      value: formatNumber(endpoint.inbound_received, locale),
    },
    {
      key: "sent",
      term: t("extOpenchannel.endpoint.sent"),
      value: formatNumber(endpoint.outbound_sent, locale),
    },
  ];
  return (
    <>
      <p>
        {endpoint.enabled ? (
          <Badge tone="success">{t("extOpenchannel.endpoint.enabled")}</Badge>
        ) : (
          <Badge tone="warn">{t("extOpenchannel.endpoint.paused")}</Badge>
        )}
      </p>
      <FactList numeric facts={facts} />
      <Recipe endpoint={endpoint} />
      {canChange ? <EndpointControls endpoint={endpoint} /> : null}
    </>
  );
}

/** The three writes a seat holding `update` may make. */
function EndpointControls({ endpoint }: Readonly<{ endpoint: Endpoint }>) {
  return (
    <>
      <SigningSecret endpointId={endpoint.id} />
      <PauseResume enabled={endpoint.enabled} />
      <OutboundUrlForm
        key={endpoint.url}
        endpointId={endpoint.id}
        url={endpoint.url}
      />
    </>
  );
}

/**
 * Minting, and the one moment the secret exists on a screen.
 *
 * It is held in this component's own state and nowhere else: no operation
 * reads a secret back, so what is on screen after a mint is the only copy that
 * will ever be offered. The sentence saying so sits BESIDE the value, at the
 * moment it is shown — a reader who has to hover to learn that has already
 * navigated away.
 */
function SigningSecret({ endpointId }: Readonly<{ endpointId: string }>) {
  const t = useT();
  const refresh = useRefreshEverything();
  const [secret, setSecret] = useState<string | null>(null);
  const mint = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await api.POST(
        "/ext/openchannel/endpoint/secret",
        { body: { endpoint_id: endpointId } },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      const minted: Minted | undefined = data;
      if (typeof minted?.signing_secret !== "string") {
        throw new Error("the mint carried no `signing_secret` field");
      }
      return minted.signing_secret;
    },
    onSuccess: setSecret,
    onSettled: refresh,
  });
  return (
    <>
      <div className="card-actions">
        <Button
          variant="ghost"
          disabled={mint.isPending}
          onClick={() => mint.mutate()}
        >
          {t("extOpenchannel.secret.mint")}
        </Button>
      </div>
      {secret ? (
        <>
          <Callout tone="warn">{t("extOpenchannel.secret.shownOnce")}</Callout>
          <pre
            className="code-block t-mono"
            data-testid="openchannel-signing-secret"
          >
            {secret}
          </pre>
        </>
      ) : null}
      {mint.isError ? (
        <p role="alert">{t("extOpenchannel.secret.mintFailed")}</p>
      ) : null}
    </>
  );
}

/**
 * Pausing and resuming, which are exact inverses: a paused endpoint keeps its
 * owner and its sealed secret, so resuming puts every configured sender
 * straight back to work with nothing re-issued.
 */
function PauseResume({ enabled }: Readonly<{ enabled: boolean }>) {
  const t = useT();
  const refresh = useRefreshEverything();
  const setEnabled = useMutation({
    // The state to move to travels as a VARIABLE. A handler reading `enabled`
    // from the render it closed over would submit the state the screen opened
    // with after a poll moved it, which for this control means resuming an
    // endpoint the member has just paused.
    mutationFn: async (next: boolean) => {
      const { error, response } = await api.PUT(
        "/ext/openchannel/endpoint/enabled",
        { body: { enabled: next } },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
    },
    onSettled: refresh,
  });
  return (
    <>
      <div className="card-actions">
        <Button
          variant={enabled ? "danger" : "primary"}
          disabled={setEnabled.isPending}
          onClick={() => setEnabled.mutate(!enabled)}
        >
          {enabled
            ? t("extOpenchannel.endpoint.pause")
            : t("extOpenchannel.endpoint.resume")}
        </Button>
      </div>
      {setEnabled.isError ? (
        <p role="alert">{t("extOpenchannel.endpoint.enabledFailed")}</p>
      ) : null}
    </>
  );
}

/**
 * Where this connector talks back to.
 *
 * Keyed on the stored address by its caller, so a value another tab registered
 * re-seeds the field rather than leaving a stale one that the next press would
 * submit back over the new one.
 */
function OutboundUrlForm({
  endpointId,
  url,
}: Readonly<{ endpointId: string; url: string }>) {
  const t = useT();
  const refresh = useRefreshEverything();
  const [draft, setDraft] = useState(url);
  const register = useMutation({
    mutationFn: async (next: string) => {
      const { error, response } = await api.PUT(
        "/ext/openchannel/endpoint/url",
        { body: { endpoint_id: endpointId, url: next } },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
    },
    onSettled: refresh,
  });
  return (
    <>
      <Field
        label={t("extOpenchannel.outbound.urlLabel")}
        hint={t("extOpenchannel.outbound.urlHint")}
      >
        {(control) => (
          <TextInput
            {...control}
            value={draft}
            placeholder={t("extOpenchannel.outbound.urlPlaceholder")}
            onChange={(event) => setDraft(event.target.value)}
          />
        )}
      </Field>
      <div className="form-actions">
        <Button
          disabled={draft.trim() === "" || register.isPending}
          onClick={() => register.mutate(draft.trim())}
        >
          {t("extOpenchannel.outbound.register")}
        </Button>
      </div>
      {register.isError ? (
        <p role="alert">{t("extOpenchannel.outbound.registerFailed")}</p>
      ) : null}
    </>
  );
}
