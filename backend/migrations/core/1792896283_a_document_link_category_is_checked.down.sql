ALTER TABLE attachment RENAME CONSTRAINT attachment_category_check TO attachment_category_check_v2;
ALTER TABLE attachment ADD CONSTRAINT attachment_category_check
    CHECK (category IN ('contract', 'offer', 'legal', 'email_attachment', 'message_attachment', 'other')) NOT VALID;
