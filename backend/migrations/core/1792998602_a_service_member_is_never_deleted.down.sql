SET LOCAL lock_timeout = '2s';

DROP TRIGGER app_user_service_never_deleted ON app_user;
DROP FUNCTION refuse_service_member_delete();
ALTER TABLE app_user DROP CONSTRAINT app_user_service_is_a_passwordless_agent;
ALTER TABLE app_user DROP COLUMN service_extension;
