-- Remove the thirteen objects from the seeded roles. An operator who hand-set
-- one after the fact loses that setting on a revert, which is the same trade
-- every permission backfill in this tree makes.
SET LOCAL lock_timeout = '5s';

UPDATE role SET permissions = permissions
        #- '{objects,user_admin}'
        #- '{objects,role_admin}'
        #- '{objects,team_admin}'
        #- '{objects,privacy_request}'
        #- '{objects,audit_log}'
        #- '{objects,job_health}'
        #- '{objects,extension_access}'
        #- '{objects,system_reset}'
        #- '{objects,ai_diagnostics}'
        #- '{objects,consent_config}'
        #- '{objects,authentication_policy}'
        #- '{objects,oauth_application}'
        #- '{objects,seat_usage}'
    WHERE is_system AND permissions ? 'objects';
