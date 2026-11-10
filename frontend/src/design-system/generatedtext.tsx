// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  QueryClientContext,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { type ReactNode, useContext, useEffect, useRef, useState } from "react";
import type { components } from "../api/schema";
import { useLocale, useT } from "../i18n";
import {
  boundRenderingCache,
  renderingOptions,
} from "../lib/reader-rendering-query";
import { useMe } from "../lib/session";
import { Button } from "./button";
import { ProvenanceTag } from "./provenance";
import { readerRenderingRefusal } from "./readerrendering";

type Display = components["schemas"]["SystemDisplay"];

export function GeneratedText({
  activityId,
  display,
  part = "subject",
  inline = false,
  children,
  renderText,
}: Readonly<{
  activityId: string;
  display?: Display;
  part?: "subject" | "body";
  inline?: boolean;
  children: ReactNode;
  renderText?: (text: string) => ReactNode;
}>) {
  const client = useContext(QueryClientContext);
  if (
    !client ||
    !display?.source_version ||
    !display.generated_parts?.includes(part)
  )
    return <>{children}</>;
  return (
    <GeneratedPart
      key={`${activityId}:${display.source_version}:${part}`}
      activityId={activityId}
      version={display.source_version}
      part={part}
      inline={inline}
      renderText={renderText}
    >
      {children}
    </GeneratedPart>
  );
}

function GeneratedPart({
  activityId,
  version,
  part,
  inline,
  children,
  renderText,
}: Readonly<{
  activityId: string;
  version: number;
  part: "subject" | "body";
  inline: boolean;
  children: ReactNode;
  renderText?: (text: string) => ReactNode;
}>) {
  const { locale } = useLocale();
  const client = useQueryClient();
  const session = useMe();
  const t = useT();
  const element = useRef<HTMLElement | null>(null);
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    if (!element.current || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver((entries) => {
      setVisible(entries.some((entry) => entry.isIntersecting));
    });
    observer.observe(element.current);
    return () => observer.disconnect();
  }, []);
  const options = renderingOptions(client, activityId, version, locale, part);
  const result = useQuery({ ...options, enabled: options.enabled && visible });
  const [original, setOriginal] = useState(false);
  useEffect(() => {
    if (result.data) boundRenderingCache(client);
  }, [client, result.data]);
  const rendering = result.data?.rendering;
  const translated = rendering?.[part];
  const showing =
    session.data?.reader_rendering_available === true &&
    !original &&
    !result.isFetching &&
    !result.isError &&
    rendering?.target_language === locale &&
    rendering.source_version === version &&
    translated;
  const refusal = readerRenderingRefusal(result.error);
  const Block = renderText && !inline ? "div" : "span";
  return (
    <Block
      ref={(node) => {
        element.current = node;
      }}
      title={inline && typeof children === "string" ? children : undefined}
    >
      {showing ? generatedContent(translated, renderText) : children}{" "}
      {showing && !rendering?.unchanged && (
        <ProvenanceTag provenance={{ kind: "agent" }} />
      )}
      {!inline && result.data && !rendering?.unchanged && (
        <Button variant="ghost" onClick={() => setOriginal(!original)}>
          {t(
            original ? "readerRendering.show" : "readerRendering.showOriginal",
          )}
        </Button>
      )}
      {!inline &&
        session.data &&
        session.data.reader_rendering_available !== true && (
          <span className="t-caption">{t("readerRendering.noModel")}</span>
        )}
      {result.isFetching && (
        <span className="t-caption">{t("readerRendering.pending")}</span>
      )}
      {!inline && result.isError && (
        <span className="t-caption">
          {t(refusal ?? "readerRendering.showOriginal")}{" "}
          <Button
            variant="ghost"
            onClick={() => {
              void result.refetch();
            }}
          >
            {t("readerRendering.show")}
          </Button>
        </span>
      )}
    </Block>
  );
}

function generatedContent(
  text: string,
  renderText?: (text: string) => ReactNode,
): ReactNode {
  return renderText ? renderText(text) : text;
}
