-- The seats whose calendar rosters list this participant row. NULL is a row no
-- roster lists. roster_seat holds the one seat of an older row until a data task moves it here.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity_participant ADD COLUMN roster_seats uuid[];
