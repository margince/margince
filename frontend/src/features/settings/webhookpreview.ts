import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useSettledValue } from "../../design-system/debouncedsearch";
import { unwrap } from "../../lib/problem";

export type WebhookBodyContentType = NonNullable<
  components["schemas"]["WebhookBodyContentType"]
>;
export type WebhookBodyPreview = components["schemas"]["WebhookBodyPreview"];

async function renderPreview(
  eventType: string,
  template: string,
  contentType: WebhookBodyContentType,
): Promise<WebhookBodyPreview> {
  return unwrap(
    await api.POST("/webhook-subscriptions/body-preview", {
      body: {
        event_type: eventType,
        body_template: template,
        body_content_type: contentType,
      },
    }),
  );
}

// The body a delivery of a sample event would send. The server's own engine
// renders it once typing pauses, so the preview and the delivery agree.
export function useBodyPreview(
  eventType: string,
  template: string,
  contentType: WebhookBodyContentType,
) {
  // One primitive, so the settle effect sees a change only when a value does.
  const settled = JSON.parse(
    useSettledValue(JSON.stringify([eventType, template, contentType])),
  ) as [string, string, WebhookBodyContentType];
  const [settledType, settledTemplate, settledContentType] = settled;
  return useQuery({
    queryKey: ["webhook-body-preview", ...settled],
    queryFn: () =>
      renderPreview(settledType, settledTemplate, settledContentType),
    enabled: settledType !== "" && settledTemplate.trim() !== "",
    retry: false,
  });
}

// The sample `data` of one event type, read through the same preview: the
// contract builds the sample, so its fields are the fields a delivery carries.
export function useEventSampleData(eventType: string) {
  return useQuery({
    queryKey: ["webhook-event-sample", eventType],
    queryFn: async (): Promise<Record<string, unknown>> => {
      const preview = await renderPreview(
        eventType,
        "{{ data | json }}",
        "text/plain",
      );
      return JSON.parse(preview.body) as Record<string, unknown>;
    },
    enabled: eventType !== "",
    staleTime: Number.POSITIVE_INFINITY,
  });
}
