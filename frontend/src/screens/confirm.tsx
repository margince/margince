import { useMutation, useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { api } from "../api/client";
import { Button, Card, EmptyState, Skeleton } from "../design-system/atoms";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import {
  explainPublicError,
  LinkInvalidError,
  RateLimitedError,
} from "./preferences";
import "./preferences.css";
import "./confirm.css";

// The public, anonymous confirm-your-details page: what a contact lands on from
// the link we email them. The token in the URL is the whole capability — no
// session, no workspace header. Unknown, expired and already-answered tokens all
// read as absent (404), so the page is never an oracle for which it was.
//
// The ORDER on this page is load-bearing and not a layout preference. The card
// comes first and the marketing ask second, because a consent asked for before
// the disclosure that informs it is not informed consent (Art. 7), and because a
// page that leads with the ask reads as advertising rather than as the Art. 14
// notice it is. Neither answer is pre-selected: a pre-ticked box is void under
// Art. 4(11) and Recital 32, settled in Planet49.

// The fields the page shows, in the order it shows them. `company` is
// deliberately absent from the correctable set: which organization employs
// somebody is a relationship the workspace maintains, and correcting it would
// mean creating or merging a company record.
const CORRECTABLE = ["full_name", "title", "email", "phone"] as const;
type CorrectableField = (typeof CORRECTABLE)[number];

const FIELD_LABELS: Record<CorrectableField, MessageKey> = {
  full_name: "confirm.field.fullName",
  title: "confirm.field.title",
  email: "confirm.field.email",
  phone: "confirm.field.phone",
};

export function ConfirmDetailsScreen({ token }: Readonly<{ token?: string }>) {
  const t = useT();
  if (!token) {
    return (
      <div className="pref-page">
        <EmptyState>{t("confirm.invalidLink")}</EmptyState>
      </div>
    );
  }
  return <ConfirmDetailsBody token={token} />;
}

function ConfirmDetailsBody({ token }: Readonly<{ token: string }>) {
  const t = useT();

  const details = useQuery({
    queryKey: ["confirm-details", token],
    retry: false,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/public/confirm/{token}",
        {
          params: { path: { token } },
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
  });

  // Edits are held against what the server sent, so a field the person did not
  // touch is never submitted as a correction. Submitting an untouched field
  // would stage a proposal nobody made, and a rep would have to read it.
  const [edits, setEdits] = useState<Partial<Record<CorrectableField, string>>>(
    {},
  );
  const [marketing, setMarketing] = useState<"granted" | "withdrawn" | null>(
    null,
  );
  const [erasure, setErasure] = useState(false);
  const [done, setDone] = useState(false);

  const card = details.data;

  // The sentence shown beside the choice IS the sentence stored as proof, so it
  // is read once here and passed to the submit. Re-deriving it at submit time
  // is how the two drift.
  const marketingWording = t("confirm.marketing.ask");

  const corrections = useMemo(() => {
    if (!card) {
      return [];
    }
    return CORRECTABLE.filter(
      (field) => edits[field] !== undefined && edits[field] !== card[field],
    ).map((field) => ({ field, value: edits[field] ?? "" }));
  }, [card, edits]);

  const submit = useMutation({
    mutationFn: async () => {
      const { error, response } = await api.POST("/public/confirm/{token}", {
        params: { path: { token } },
        body: {
          corrections,
          request_erasure: erasure,
          ...(marketing
            ? {
                marketing_choice: marketing,
                marketing_wording: marketingWording,
              }
            : {}),
        },
      });
      if (error) {
        if (response.status === 404) {
          throw new LinkInvalidError();
        }
        if (response.status === 429) {
          throw new RateLimitedError();
        }
        throwProblem(error);
      }
    },
    onSuccess: () => setDone(true),
  });

  if (details.isPending) {
    return (
      <div className="pref-page">
        <Skeleton width="100%" />
      </div>
    );
  }
  if (details.error || !card) {
    return (
      <div className="pref-page">
        <EmptyState>{explainPublicError(details.error, t)}</EmptyState>
      </div>
    );
  }
  if (done) {
    return (
      <div className="pref-page">
        <Card>
          <h1 className="t-h2">{t("confirm.done.title")}</h1>
          <p className="t-body">{t("confirm.done.body")}</p>
        </Card>
      </div>
    );
  }

  return (
    <div className="pref-page">
      <h1 className="t-h2">{t("confirm.title")}</h1>
      <p className="t-body confirm-intro">{t("confirm.intro")}</p>

      <Card>
        <h2 className="t-h3">{t("confirm.card.title")}</h2>
        <ul className="confirm-fields">
          {CORRECTABLE.map((field) => (
            <li key={field} className="confirm-field">
              <label className="t-caption" htmlFor={`confirm-${field}`}>
                {t(FIELD_LABELS[field])}
              </label>
              <input
                id={`confirm-${field}`}
                className="confirm-input"
                value={edits[field] ?? card[field]}
                onChange={(event) =>
                  setEdits({ ...edits, [field]: event.target.value })
                }
              />
            </li>
          ))}
          <li className="confirm-field">
            <span className="t-caption">{t("confirm.field.company")}</span>
            <span className="confirm-readonly">
              {card.company || t("confirm.field.none")}
            </span>
          </li>
        </ul>
      </Card>

      <Card>
        <h2 className="t-h3">{t("confirm.marketing.title")}</h2>
        <p className="t-body">{marketingWording}</p>
        <div className="confirm-choices">
          <Button
            variant={marketing === "granted" ? "primary" : "ghost"}
            aria-pressed={marketing === "granted"}
            onClick={() => setMarketing("granted")}
          >
            {t("confirm.marketing.yes")}
          </Button>
          <button
            type="button"
            className="confirm-decline"
            aria-pressed={marketing === "withdrawn"}
            onClick={() => setMarketing("withdrawn")}
          >
            {t("confirm.marketing.no")}
          </button>
        </div>
      </Card>

      <details className="confirm-provenance">
        <summary className="t-caption">{t("confirm.provenance.title")}</summary>
        {card.provenance.length === 0 ? (
          <p className="t-caption">{t("confirm.provenance.empty")}</p>
        ) : (
          <ul>
            {card.provenance.map((origin) => (
              <li
                key={`${origin.field}-${origin.recorded_at}`}
                className="t-caption"
              >
                {t("confirm.provenance.line", {
                  field: origin.field,
                  source: origin.source,
                  date: origin.recorded_at,
                })}
              </li>
            ))}
          </ul>
        )}
      </details>

      {submit.error && (
        <p className="t-caption confirm-error">
          {explainPublicError(submit.error, t)}
        </p>
      )}

      <div className="confirm-actions">
        <Button
          variant="primary"
          disabled={submit.isPending}
          onClick={() => submit.mutate()}
        >
          {t("confirm.submit")}
        </Button>
        <button
          type="button"
          className="confirm-erasure"
          aria-pressed={erasure}
          onClick={() => setErasure(!erasure)}
        >
          {erasure ? t("confirm.erasure.staged") : t("confirm.erasure.ask")}
        </button>
      </div>
    </div>
  );
}
