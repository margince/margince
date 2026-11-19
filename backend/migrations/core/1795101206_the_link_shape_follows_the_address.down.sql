-- The old shape allows an address only on a `link` row, so a link a user
-- added is kept as one rather than deleted.
SET LOCAL lock_timeout = '3s';

UPDATE attachment SET category = 'link' WHERE url IS NOT NULL AND category IS DISTINCT FROM 'link';
ALTER TABLE attachment RENAME CONSTRAINT attachment_link_shape TO attachment_link_shape_v2;
ALTER TABLE attachment ADD CONSTRAINT attachment_link_shape
    CHECK ((category = 'link') = (url IS NOT NULL)
       AND (category <> 'link'
            OR (storage_key = '' AND bytes_withheld AND url ~ '^https?://' AND length(url) <= 2048))) NOT VALID;
