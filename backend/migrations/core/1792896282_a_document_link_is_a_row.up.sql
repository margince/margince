-- A document link sent by mail (a Google Docs or Drive share) is an attachment
-- row of the category `link`. It has no bytes, so it reuses bytes_withheld: every
-- reader that fetches bytes already asks that flag before it touches
-- storage_key, and attachment_link_shape keeps the pairing from being broken.
--
-- Metadata-only: a nullable column and two constraints added NOT VALID. The next
-- migration validates them in its own transaction and swaps the category check.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment ADD COLUMN url text;

ALTER TABLE attachment ADD CONSTRAINT attachment_category_check_v2
    CHECK (category IN ('contract', 'offer', 'legal', 'email_attachment', 'message_attachment', 'other', 'link')) NOT VALID;

ALTER TABLE attachment ADD CONSTRAINT attachment_link_shape
    CHECK ((category = 'link') = (url IS NOT NULL)
       AND (category <> 'link'
            OR (storage_key = '' AND bytes_withheld AND url ~ '^https?://' AND length(url) <= 2048))) NOT VALID;
