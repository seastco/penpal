ALTER TABLE stamps DROP CONSTRAINT stamps_earned_via_check;
ALTER TABLE stamps ADD CONSTRAINT stamps_earned_via_check
    CHECK (earned_via IN ('registration', 'delivery', 'route', 'transfer', 'weekly', 'mint'));
