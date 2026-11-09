-- pgmigrate:no-transaction
-- A sign-in reads a seat by its login in either spelling of the domain
-- (storekit.SeatLoginIsSQL). The raw spelling is found through uq_app_user_email.
-- The folded arm compares the domain the writers fold and the lowercased local
-- part, and every seat at one company shares that domain, so it read every seat.
-- The expressions are the predicate's own, or the planner cannot use them. The
-- first drop clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_app_user_login_folded;
CREATE INDEX CONCURRENTLY idx_app_user_login_folded
    ON app_user (email_domain_folded, lower(regexp_replace(email, '@[^@]*$', '')));
