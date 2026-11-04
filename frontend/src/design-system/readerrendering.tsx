// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One note, mail or meeting shown in the reader's own language on a press.
//
// The rendering is for this reader alone and the server stores none of it. So
// it lives in this component's state, not a query cache, and is gone when the
// row unmounts. That is also why a timeline row needs no query client for it.

import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemCodeOf, unwrap } from "../lib/problem";
import { Button } from "./button";
import { ErrorLine } from "./errorline";
import { ProvenanceTag } from "./provenance";
import "./readerrendering.css";

export type ActivityReaderRendering =
  components["schemas"]["ActivityReaderRendering"];
export type MessageLanguage = "de" | "en" | "vi";

type Press =
  | { state: "idle" }
  | { state: "pending" }
  | { state: "answered"; rendering: ActivityReaderRendering }
  | { state: "failed"; error: unknown };

/**
 * The press and where it stands. `press` takes what it asks for as arguments,
 * so a click can never send the language of a render before it.
 */
export function useReaderRendering() {
  const [press, setPress] = useState<Press>({ state: "idle" });
  const ask = async (activityId: string, locale: Locale) => {
    setPress({ state: "pending" });
    try {
      const rendering = unwrap(
        await api.POST("/activities/{id}/reader-rendering", {
          params: { path: { id: activityId } },
          body: { locale },
        }),
      );
      setPress({ state: "answered", rendering });
    } catch (error) {
      setPress({ state: "failed", error });
    }
  };
  return { press, ask };
}

/**
 * Whether a message offers the press: one in another language than the
 * reader's, or one whose language is not known. A message known to be in the
 * reader's language has nothing to show.
 */
export function offersReaderRendering(
  language: MessageLanguage | null | undefined,
  locale: Locale,
): boolean {
  return language !== locale;
}

/** The refusals a reader gets its own sentence for; the rest are generic. */
const REFUSAL_COPY: Readonly<Record<string, MessageKey>> = {
  ai_not_configured: "readerRendering.noModel",
  too_long: "readerRendering.tooLong",
};

/** The sentence for a refused press, or null for the generic failure line. */
export function readerRenderingRefusal(error: unknown): MessageKey | null {
  const code = problemCodeOf(error);
  return (code && REFUSAL_COPY[code]) || null;
}

/**
 * The press and its result around the original text. `children` is the
 * original as the surface draws it; the rendering takes its place while shown.
 */
export function ReaderRendering({
  activityId,
  language,
  children,
}: Readonly<{
  activityId: string;
  language: MessageLanguage | null | undefined;
  children: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { press, ask } = useReaderRendering();
  const [shown, setShown] = useState(false);
  if (!offersReaderRendering(language, locale)) {
    return <>{children}</>;
  }
  const answer = press.state === "answered" ? press.rendering : undefined;
  const showing =
    shown && answer && !answer.unchanged && answer.target_language === locale;
  return (
    <div className="readerrendering">
      {showing ? (
        <RenderedText rendering={answer} onOriginal={() => setShown(false)} />
      ) : (
        children
      )}
      {!showing && (
        <div className="readerrendering__ask">
          <Button
            variant="ghost"
            pending={press.state === "pending"}
            busyLabel={t("readerRendering.pending")}
            onClick={() => {
              setShown(true);
              void ask(activityId, locale);
            }}
          >
            {t("readerRendering.show")}
          </Button>
          {answer?.unchanged && (
            <span className="readerrendering__note">
              {t("readerRendering.alreadyYours")}
            </span>
          )}
        </div>
      )}
      <RenderingFailure error={press.state === "failed" ? press.error : null} />
    </div>
  );
}

function RenderedText({
  rendering,
  onOriginal,
}: Readonly<{ rendering: ActivityReaderRendering; onOriginal: () => void }>) {
  const t = useT();
  return (
    <section
      className="readerrendering__text"
      aria-label={t("readerRendering.label")}
    >
      <div className="readerrendering__head">
        <ProvenanceTag provenance={{ kind: "agent" }} />
        <span className="readerrendering__note">
          {t("readerRendering.disclaimer")}
        </span>
      </div>
      {rendering.subject && (
        <p className="readerrendering__subject">{rendering.subject}</p>
      )}
      {rendering.body && (
        <p className="readerrendering__body">{rendering.body}</p>
      )}
      <Button variant="ghost" onClick={onOriginal}>
        {t("readerRendering.showOriginal")}
      </Button>
    </section>
  );
}

function RenderingFailure({ error }: Readonly<{ error: unknown }>) {
  const t = useT();
  if (!error) {
    return null;
  }
  const refusal = readerRenderingRefusal(error);
  return refusal ? (
    <ErrorLine>{t(refusal)}</ErrorLine>
  ) : (
    <ErrorLine error={error} />
  );
}
