SET LOCAL lock_timeout = '3s';

DROP TRIGGER trg_signal_updated ON signal;
CREATE TRIGGER trg_signal_updated
	BEFORE UPDATE ON signal
	FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();
