-- Track which stamp types a user has ever owned (discovered).
CREATE TABLE stamp_discoveries (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stamp_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, stamp_type)
);

-- Backfill from current ownership.
INSERT INTO stamp_discoveries (user_id, stamp_type, created_at)
SELECT DISTINCT owner_id, stamp_type, MIN(created_at)
FROM stamps
WHERE owner_id IS NOT NULL
GROUP BY owner_id, stamp_type
ON CONFLICT DO NOTHING;
