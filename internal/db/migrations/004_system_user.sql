-- 004_system_user.sql: Create the penpal#0000 system user for welcome letters

INSERT INTO users (id, username, discriminator, public_key, home_city, home_lat, home_lng)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'penpal',
    '0000',
    decode('7d4fedc7813f50c97413d24a230dca5899ad3efa969fe44d01a4ed4f5857ea19', 'hex'),
    'Green Bay, WI',
    44.5133,
    -88.0133
) ON CONFLICT DO NOTHING;
