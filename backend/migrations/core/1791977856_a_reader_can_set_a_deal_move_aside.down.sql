-- Back to a deal card whose move cannot be set aside. The rows are view state,
-- so dropping them loses only which moves readers had put away.
SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS deal_move_dismissal;
