import type { WebhookBodyContentType } from "./webhookpreview";

// Starting points for a custom body. Each is valid for the server's JSON
// rule: a placeholder inside quotes fills text, `| json` writes a whole value.
export const WEBHOOK_BODY_PRESETS = {
  slack: {
    contentType: "application/json",
    template: `{
  "text": "{{ type }} on {{ entity.type }} {{ entity.id }}",
  "blocks": [
    {
      "type": "section",
      "text": { "type": "mrkdwn", "text": "*{{ type }}*\\nRecord {{ entity.id }}" }
    },
    {
      "type": "context",
      "elements": [{ "type": "mrkdwn", "text": "{{ occurred_at }}" }]
    }
  ]
}`,
  },
  teams: {
    contentType: "application/json",
    template: `{
  "type": "message",
  "attachments": [
    {
      "contentType": "application/vnd.microsoft.card.adaptive",
      "content": {
        "type": "AdaptiveCard",
        "version": "1.4",
        "body": [
          { "type": "TextBlock", "weight": "Bolder", "text": "{{ type }}" },
          { "type": "TextBlock", "wrap": true, "text": "Record {{ entity.type }} {{ entity.id }}" },
          { "type": "TextBlock", "isSubtle": true, "text": "{{ occurred_at }}" }
        ]
      }
    }
  ]
}`,
  },
  generic: {
    contentType: "application/json",
    template: `{
  "event": "{{ type }}",
  "id": "{{ event_id }}",
  "at": "{{ occurred_at }}",
  "record": { "type": "{{ entity.type }}", "id": "{{ entity.id }}" },
  "data": {{ data | json }}
}`,
  },
} as const satisfies Record<
  string,
  { contentType: WebhookBodyContentType; template: string }
>;

export type WebhookBodyPreset = keyof typeof WEBHOOK_BODY_PRESETS;

export const WEBHOOK_BODY_CONTENT_TYPES: readonly WebhookBodyContentType[] = [
  "application/json",
  "text/plain",
  "application/x-www-form-urlencoded",
];

// The envelope fields every event carries; `data.*` comes from the sample.
export const ENVELOPE_VARIABLES = [
  "type",
  "event_id",
  "occurred_at",
  "entity.type",
  "entity.id",
  "entity.name",
  "entity.owner_name",
  "entity.url",
  "actor.type",
  "correlation_id",
  "version",
] as const;
