-- Rename shipping tier "first_class" to "standard"
UPDATE messages SET shipping_tier = 'standard' WHERE shipping_tier = 'first_class';
ALTER TABLE messages DROP CONSTRAINT messages_shipping_tier_check;
ALTER TABLE messages ADD CONSTRAINT messages_shipping_tier_check
    CHECK (shipping_tier IN ('standard', 'priority', 'express'));
