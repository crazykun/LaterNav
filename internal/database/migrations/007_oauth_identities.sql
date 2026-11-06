CREATE TABLE IF NOT EXISTS oauth_identities (
    provider VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    subject VARBINARY(255) NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    preferred_username VARCHAR(255) NOT NULL DEFAULT '',
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    picture VARCHAR(2048) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    PRIMARY KEY (provider, subject),
    UNIQUE KEY uk_oauth_user_provider (user_id, provider),
    CONSTRAINT fk_oauth_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
