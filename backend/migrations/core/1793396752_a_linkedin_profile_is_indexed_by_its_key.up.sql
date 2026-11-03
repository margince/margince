-- pgmigrate:no-transaction
-- The index the LinkedIn duplicate lane finds a profile's holders by: the slug
-- under /in/, lower case, so a localized host or a trailing slash is the same
-- profile. Built concurrently, so writes are never blocked, and the first drop
-- clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_social_linkedin_profile;
CREATE INDEX CONCURRENTLY idx_contact_social_linkedin_profile ON contact_social
    ((substring(lower(handle) from '^(?:https?://)?(?:[a-z0-9-]+\.)*linkedin\.com/in/([^/?#]+)')))
    WHERE platform = 'linkedin';
