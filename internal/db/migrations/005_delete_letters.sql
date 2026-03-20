-- 005_delete_letters.sql: Add soft-delete columns for per-user letter deletion

ALTER TABLE messages ADD COLUMN sender_deleted BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE messages ADD COLUMN recipient_deleted BOOLEAN NOT NULL DEFAULT false;
