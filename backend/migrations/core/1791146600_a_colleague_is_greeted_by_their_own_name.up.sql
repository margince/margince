-- The name a colleague is greeted by, when the first word of their display name
-- is the wrong word: "Dr. Sofia Meier" was greeted "Hi Dr.," and "Nguyễn Thị
-- Lan" "Chào Nguyễn". NULL means nobody has said, and the greeting falls back
-- to that first word (draftfloor.GreetingName).
--
-- The bound is the contract's, counted in characters like display_name's. The
-- CHECK sits on the column it arrives with: the scan it costs reads a table of
-- seats in which every row holds NULL.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user
    ADD COLUMN greeting_name text
        CONSTRAINT app_user_greeting_name_length
        CHECK (greeting_name IS NULL OR char_length(greeting_name) BETWEEN 1 AND 100);
