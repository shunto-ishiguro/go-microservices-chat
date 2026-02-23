CREATE TABLE IF NOT EXISTS users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cognito_sub  VARCHAR(255) UNIQUE,
    email        VARCHAR(255) UNIQUE NOT NULL,
    username     VARCHAR(50) UNIQUE NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    avatar_url   VARCHAR(500) DEFAULT '',
    status_text  VARCHAR(200) DEFAULT '',
    is_online    BOOLEAN DEFAULT false,
    last_seen_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_cognito_sub ON users(cognito_sub);
