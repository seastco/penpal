-- 002_stamp_consumption.sql: Allow stamps to be consumed on send.
-- Stamps get owner_id = NULL while in transit, transferred to recipient on delivery.
-- The unique constraint is relaxed so a stamp can be re-sent after being received.

ALTER TABLE stamps ALTER COLUMN owner_id DROP NOT NULL;

ALTER TABLE stamp_attachments DROP CONSTRAINT stamp_attachments_stamp_id_key;

ALTER TABLE stamp_attachments
  ADD CONSTRAINT stamp_attachments_message_stamp_unique UNIQUE(message_id, stamp_id);
