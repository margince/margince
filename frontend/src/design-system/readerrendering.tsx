// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClientContext } from "@tanstack/react-query";
import {
  type ReactNode,
  useContext,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { healthScope } from "../lib/healthscopekey";
import { problemCodeOf, unwrap } from "../lib/problem";
import { Button } from "./button";
import { ErrorLine } from "./errorline";
import { NoteMarkdown } from "./markdown";
import { ProvenanceTag } from "./provenance";
import "./readerrendering.css";

export type ActivityReaderRendering =
  components["schemas"]["ActivityReaderRendering"];
export type MessageLanguage = "de" | "en" | "vi";

type Press =
  | { state: "idle" }
  | { state: "pending"; scope?: string }
  | {
      state: "answered";
      rendering: ActivityReaderRendering;
      scope?: string;
      version?: number;
    }
  | { state: "failed"; error: unknown; scope?: string };

/**
 * The press and where it stands. `press` takes what it asks for as arguments,
 * so a click can never send the language of a render before it.
 */
export function useReaderRendering() {
  const client = useContext(QueryClientContext);
  const scope = useSyncExternalStore(
    (notify) =>
      client
        ? client.getQueryCache().subscribe((event) => {
            if (event.query.queryKey[0] === "me") notify();
          })
        : () => {},
    () =>
      healthScope(
        client?.getQueryData<components["schemas"]["MeResponse"]>(["me"]),
      ),
    () => undefined,
  );
  const [press, setPress] = useState<Press>({ state: "idle" });
  const generation = useRef(0);
  const pending = useRef<
    { controller: AbortController; scope?: string } | undefined
  >(undefined);
  useEffect(() => {
    if (pending.current && pending.current.scope !== scope) {
      generation.current++;
      pending.current.controller.abort();
      pending.current = undefined;
    }
  }, [scope]);
  useEffect(
    () => () => {
      generation.current++;
      pending.current?.controller.abort();
    },
    [],
  );
  const ask = async (activityId: string, locale: Locale, version?: number) => {
    const request = ++generation.current;
    pending.current?.controller.abort();
    const controller = new AbortController();
    pending.current = { controller, scope };
    setPress({ state: "pending", scope });
    try {
      const rendering = unwrap(
        await api.POST("/activities/{id}/reader-rendering", {
          params: { path: { id: activityId } },
          signal: controller.signal,
          body: {
            locale,
            ...(version === undefined ||
            client?.getQueryData<components["schemas"]["MeResponse"]>(["me"])
              ?.reader_rendering_available === undefined
              ? {}
              : { expected_source_version: version }),
          },
        }),
      );
      if (request === generation.current)
        setPress({ state: "answered", rendering, scope, version });
    } catch (error) {
      if (request === generation.current)
        setPress({ state: "failed", error, scope });
    }
  };
  return {
    press:
      press.state !== "idle" && press.scope !== scope
        ? ({ state: "idle" } satisfies Press)
        : press,
    ask,
  };
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
  sourceVersion,
  note = false,
  children,
}: Readonly<{
  activityId: string;
  language: MessageLanguage | null | undefined;
  sourceVersion?: number;
  /** The original is a note's markdown, so the rendering reads as one too:
   *  through NoteMarkdown, the way the original is drawn. A mail stays text. */
  note?: boolean;
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
    shown &&
    answer &&
    answer.activity_id === activityId &&
    (sourceVersion === undefined ||
      (press.state === "answered" &&
        press.version === sourceVersion &&
        (answer.source_version === undefined ||
          answer.source_version === sourceVersion))) &&
    !answer.unchanged &&
    answer.target_language === locale;
  return (
    <div className="readerrendering">
      {showing ? (
        <RenderedText
          rendering={answer}
          note={note}
          onOriginal={() => setShown(false)}
        />
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
              void ask(activityId, locale, sourceVersion);
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
  note,
  onOriginal,
}: Readonly<{
  rendering: ActivityReaderRendering;
  note: boolean;
  onOriginal: () => void;
}>) {
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
      {rendering.body &&
        (note ? (
          <div className="readerrendering__body">
            <NoteMarkdown source={rendering.body} />
          </div>
        ) : (
          <p className="readerrendering__body">{rendering.body}</p>
        ))}
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
