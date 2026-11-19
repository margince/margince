-- A link a user adds to a record keeps the category they chose, so an address
-- pairs with a row that holds no bytes rather than with the `link` category,
-- which still requires one. Added NOT VALID; the next migration validates it
-- and lets it take the old constraint's name.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment ADD CONSTRAINT attachment_link_shape_v2
    CHECK ((category <> 'link' OR url IS NOT NULL)
       AND (url IS NULL
            OR (storage_key = '' AND bytes_withheld AND url ~ '^https?://' AND length(url) <= 2048))) NOT VALID;
