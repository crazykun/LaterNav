-- 多三方登录提供商注册表。
-- identity_key 是 oauth_identities.provider 使用的稳定身份键（哈希，与 slug 解耦，
-- 改 slug/显示名不影响存量绑定）。slug 仅用于路由与管理展示。
CREATE TABLE IF NOT EXISTS oauth_providers (
    slug VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    name VARCHAR(64) NOT NULL DEFAULT '',
    type VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'oidc',
    identity_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    issuer VARCHAR(255) NOT NULL DEFAULT '',
    discovery_url VARCHAR(255) NOT NULL DEFAULT '',
    authorize_url VARCHAR(255) NOT NULL DEFAULT '',
    token_url VARCHAR(255) NOT NULL DEFAULT '',
    userinfo_url VARCHAR(255) NOT NULL DEFAULT '',
    client_id VARCHAR(255) NOT NULL DEFAULT '',
    client_secret VARCHAR(255) NOT NULL DEFAULT '',
    redirect_url VARCHAR(255) NOT NULL DEFAULT '',
    scopes VARCHAR(255) NOT NULL DEFAULT '',
    enabled TINYINT(1) NOT NULL DEFAULT 0,
    sort INT NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_provider_identity (identity_key),
    KEY idx_enabled_sort (enabled, sort)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
