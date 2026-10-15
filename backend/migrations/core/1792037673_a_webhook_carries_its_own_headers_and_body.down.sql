SET LOCAL lock_timeout = '3s';

ALTER TABLE webhook_delivery DROP COLUMN content_type;

ALTER TABLE webhook_subscription
	DROP COLUMN body_content_type,
	DROP COLUMN body_template,
	DROP COLUMN custom_headers;
