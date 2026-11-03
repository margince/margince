SET LOCAL lock_timeout = '3s';

ALTER TABLE web_form VALIDATE CONSTRAINT web_form_privacy_notice_bounded;
ALTER TABLE web_form VALIDATE CONSTRAINT web_form_privacy_notice_url_https;
