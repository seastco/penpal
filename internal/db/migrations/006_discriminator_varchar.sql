-- Allow variable-length discriminators (3 digits for new users, existing 4-digit preserved).
ALTER TABLE users ALTER COLUMN discriminator TYPE VARCHAR(4);
