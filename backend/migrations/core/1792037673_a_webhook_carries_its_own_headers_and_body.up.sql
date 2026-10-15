-- A subscription can carry its own headers and shape its own body, so a
-- receiver that wants a token or a body of its own shape can be wired.

-- Bounded, because ALTER TABLE takes a lock that blocks the deliverer's writes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE webhook_subscription
	ADD COLUMN custom_headers jsonb NOT NULL DEFAULT '[]'::jsonb,
	ADD COLUMN body_template text,
	ADD COLUMN body_content_type text;

COMMENT ON COLUMN webhook_subscription.custom_headers IS
	'Headers sent with every delivery, as [{name, sealed_value}]. Each value is '
	'sealed under the webhook key and bound to this subscription and header name.';
COMMENT ON COLUMN webhook_subscription.body_template IS
	'The template each delivery body is rendered from at enqueue; NULL sends the standard envelope.';
COMMENT ON COLUMN webhook_subscription.body_content_type IS
	'The Content-Type of a templated body; NULL means application/json.';

ALTER TABLE webhook_delivery
	ADD COLUMN content_type text,
	ADD COLUMN claimed_until timestamptz;

COMMENT ON COLUMN webhook_delivery.content_type IS
	'The Content-Type of the stored payload, fixed at enqueue with it; NULL means application/json.';
COMMENT ON COLUMN webhook_delivery.claimed_until IS
	'While in the future, one sweep or replay is sending this delivery and no other may. '
	'Recording the outcome clears it; a claim left by a crashed worker runs out.';
