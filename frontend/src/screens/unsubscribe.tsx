import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Lock, Mail } from "lucide-react";
import { useRef, useState } from "react";
import { api } from "../api/client";
import { Button, Card, PendingBody } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import {
  explainPublicError,
  LinkInvalidError,
  RateLimitedError,
} from "./preferences";
import { labelOf, type PurposeView } from "./preferences.logic";
import { PublicPage } from "./publicpage";
import "./preferences.css";

// The human unsubscribe page: where the VISIBLE "Unsubscribe" link in an
// outgoing message lands.
//
// It exists because that link used to point at the RFC 8058 endpoint,
// which is POST-only by design — so a person clicking it in their mail
// client got 405. That endpoint is still the right answer for a mailbox
// provider, which POSTs it without a browser. A person needs a page.
//
// The page does NOT unsubscribe on arrival. Mail scanners and link
// prefetchers follow links in a mailbox with no human involved, so a GET
// that withdrew consent would unsubscribe people who never clicked
// anything. One explicit press does it.
export function UnsubscribeScreen({
  token,
  purpose,
}: Readonly<{ token?: string; purpose?: string }>) {
  const t = useT();
  if (!token || !purpose) {
    return (
      <PublicPage>
        <Callout tone="warn">{t("prefs.invalidLink")}</Callout>
      </PublicPage>
    );
  }
  return <UnsubscribeBody token={token} purpose={purpose} />;
}

function UnsubscribeBody({
  token,
  purpose,
}: Readonly<{ token: string; purpose: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const queryKey = ["preference-center", token];
  const doneHeading = useRef<HTMLHeadingElement>(null);
  // null until the press answers. The empty array is NOT the same as null:
  // it is the server saying nothing moved, which is how a replay is told
  // apart from a first press.
  const [stopped, setStopped] = useState<string[] | null>(null);

  const center = useQuery({
    queryKey,
    retry: false,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/public/preferences/{token}",
        { params: { path: { token } } },
      );
      if (error) {
        if (response.status === 404) {
          throw new LinkInvalidError();
        }
        if (response.status === 429) {
          throw new RateLimitedError();
        }
        throwProblem(error);
      }
      return data;
    },
  });

  // The purpose key travels as a mutation VARIABLE rather than through the
  // closure: a passive effect can re-run this with a stale capture, and the
  // one thing that must not go stale is which purpose is being withdrawn.
  const unsubscribe = useMutation({
    mutationFn: async (purposeKey: string) => {
      const { data, error, response } = await api.POST(
        "/public/preferences/{token}/unsubscribe",
        {
          params: { path: { token }, query: { purpose: purposeKey } },
        },
      );
      if (error) {
        if (response.status === 404) {
          throw new LinkInvalidError();
        }
        if (response.status === 429) {
          throw new RateLimitedError();
        }
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => {
      setStopped(data?.unsubscribed ?? []);
      queryClient.invalidateQueries({ queryKey });
      // Focus follows the outcome, so a screen-reader user is told what
      // happened instead of being left on a button that is now gone.
      requestAnimationFrame(() => doneHeading.current?.focus());
    },
  });

  if (center.isPending) {
    return (
      <PublicPage>
        <Card>
          <PendingBody label={t("prefs.unsub.loading")} lines={4} />
        </Card>
      </PublicPage>
    );
  }
  if (center.error instanceof LinkInvalidError) {
    // WITHDRAW-ONLY, not dead. A withdrawal credential outlives the preference
    // token that rode with it, and it carries no authority to READ a consent
    // state — so this page's opening fetch 404s for exactly the links that
    // still work. Treating that as a dead end would put "this link is no
    // longer valid" in front of the person whose link we just fixed.
    //
    // The press is its own authority: the POST resolves the credential itself
    // and refuses it if it is genuinely dead. So the button stays, and what
    // goes is everything that needed the read — the purpose label, the locked
    // explanation, and the link to a preference centre this token cannot open.
    return (
      <WithdrawOnlyBody
        purpose={purpose}
        stopped={stopped}
        doneHeading={doneHeading}
        unsubscribe={unsubscribe}
      />
    );
  }
  if (center.error) {
    // A rate limit or a network failure is a "not now", and a reader who came
    // here to stop an email deserves a way to try again rather than a dead end.
    const retryable = true;
    return (
      <PublicPage>
        <Callout
          tone={center.error instanceof RateLimitedError ? "warn" : "danger"}
          actions={
            retryable ? (
              <Button onClick={() => center.refetch()}>
                {t("prefs.unsub.retry")}
              </Button>
            ) : undefined
          }
        >
          {explainPublicError(center.error, t)}
        </Callout>
      </PublicPage>
    );
  }

  const target = center.data?.purposes.find(
    (p: PurposeView) => p.key === purpose,
  );
  if (!target) {
    return (
      <PublicPage>
        <Callout tone="warn">{t("prefs.unsub.unknownPurpose")}</Callout>
        <ManageLink token={token} label={t("prefs.unsub.seeAll")} />
      </PublicPage>
    );
  }

  if (stopped !== null) {
    return (
      <StoppedBody stopped={stopped} heading={doneHeading}>
        {stopped.length > 0 && (
          <p>{t("prefs.unsub.doneBody", { label: labelOf(t, target) })}</p>
        )}
        <ManageLink token={token} label={t("prefs.unsub.manage")} />
      </StoppedBody>
    );
  }

  // A locked purpose carries no button at all. A control that always fails
  // is worse than an absent one, and here the honest answer is why.
  if (target.locked) {
    return (
      <PublicPage>
        <h1 className="t-display">{t("prefs.unsub.lockedTitle")}</h1>
        <Card>
          <p className="unsub-kind">
            <Lock size={16} aria-hidden="true" /> {labelOf(t, target)}
          </p>
          <p>{t("prefs.unsub.lockedBody")}</p>
        </Card>
        <ManageLink token={token} label={t("prefs.unsub.seeAll")} />
      </PublicPage>
    );
  }

  return (
    <PublicPage>
      <h1 className="t-display">{t("prefs.unsub.title")}</h1>
      <p className="unsub-lead">{t("prefs.unsub.lead")}</p>
      <Card>
        <p className="unsub-kind">
          <Mail size={16} aria-hidden="true" /> {labelOf(t, target)}
        </p>
        <Callout tone="info" title={t("prefs.unsub.afterTitle")}>
          {t("prefs.unsub.afterBody")}
        </Callout>
        <div className="unsub-actions">
          <Button
            variant="primary"
            pending={unsubscribe.isPending}
            busyLabel={t("prefs.unsub.busy")}
            onClick={() => unsubscribe.mutate(target.key)}
          >
            {t("prefs.unsub.confirm")}
          </Button>
          <a className="link-button" href={`#/preferences/${token}`}>
            {t("prefs.unsub.seeAll")}
          </a>
        </div>
        {unsubscribe.error ? (
          <Callout
            tone={
              unsubscribe.error instanceof RateLimitedError ? "warn" : "danger"
            }
          >
            {explainPublicError(unsubscribe.error, t)}
          </Callout>
        ) : null}
      </Card>
      <p className="unsub-privacy">{t("prefs.unsub.privacy")}</p>
    </PublicPage>
  );
}

// StoppedBody is the outcome panel both bodies land on, so the two cannot
// drift into telling a person two different things about the same press.
//
// The empty array is NOT the same as a stop: it is the server saying nothing
// moved, which is how a replay is told apart from a first press.
function StoppedBody({
  stopped,
  heading,
  children,
}: Readonly<{
  stopped: string[];
  heading: React.RefObject<HTMLHeadingElement | null>;
  children?: React.ReactNode;
}>) {
  const t = useT();
  return (
    <PublicPage>
      <Card>
        <div className="unsub-done">
          <span className="unsub-done-mark" aria-hidden="true">
            <Check size={22} />
          </span>
          <h1 ref={heading} tabIndex={-1}>
            {stopped.length > 0
              ? t("prefs.unsub.doneTitle")
              : t("prefs.unsub.alreadyOff")}
          </h1>
          {children}
        </div>
      </Card>
    </PublicPage>
  );
}

// WithdrawOnlyBody is the page a link can draw when it may stop mail and may
// not read anything.
//
// It states the purpose by KEY rather than by label, because the label lives
// behind the read this credential does not have. A key is worse copy than a
// label and better than a page that will not load.
function WithdrawOnlyBody({
  purpose,
  stopped,
  doneHeading,
  unsubscribe,
}: Readonly<{
  purpose: string;
  stopped: string[] | null;
  doneHeading: React.RefObject<HTMLHeadingElement | null>;
  unsubscribe: {
    mutate: (key: string) => void;
    isPending: boolean;
    error: unknown;
  };
}>) {
  const t = useT();
  if (stopped !== null) {
    return <StoppedBody stopped={stopped} heading={doneHeading} />;
  }
  return (
    <PublicPage>
      <h1 className="t-display">{t("prefs.unsub.title")}</h1>
      <p className="unsub-lead">{t("prefs.unsub.lead")}</p>
      <Card>
        <p className="unsub-kind">
          <Mail size={16} aria-hidden="true" /> {purpose}
        </p>
        <Callout tone="info" title={t("prefs.unsub.afterTitle")}>
          {t("prefs.unsub.afterBody")}
        </Callout>
        <div className="unsub-actions">
          <Button
            variant="primary"
            pending={unsubscribe.isPending}
            busyLabel={t("prefs.unsub.busy")}
            onClick={() => unsubscribe.mutate(purpose)}
          >
            {t("prefs.unsub.confirm")}
          </Button>
        </div>
        {unsubscribe.error ? (
          <Callout
            tone={
              unsubscribe.error instanceof RateLimitedError ? "warn" : "danger"
            }
          >
            {explainPublicError(unsubscribe.error, t)}
          </Callout>
        ) : null}
      </Card>
      <p className="unsub-privacy">{t("prefs.unsub.privacy")}</p>
    </PublicPage>
  );
}

function ManageLink({
  token,
  label,
}: Readonly<{ token: string; label: string }>) {
  return (
    <p className="unsub-actions">
      <a className="link-button" href={`#/preferences/${token}`}>
        {label}
      </a>
    </p>
  );
}
