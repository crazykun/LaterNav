-- 会员投稿站点表。status: pending 待审核 / approved 已通过 / rejected 已驳回。
-- 审核通过后内容写入 sites 表，本记录保留并回链 site_id 供追溯。
CREATE TABLE IF NOT EXISTS site_submissions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    name VARCHAR(150) NOT NULL,
    url VARCHAR(512) NOT NULL,
    description TEXT,
    logo VARCHAR(512) NOT NULL DEFAULT '',
    category VARCHAR(100) NOT NULL DEFAULT '',
    tags VARCHAR(512) NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    reject_reason VARCHAR(500) NOT NULL DEFAULT '',
    reviewed_by BIGINT UNSIGNED DEFAULT NULL,
    reviewed_at DATETIME DEFAULT NULL,
    site_id BIGINT UNSIGNED DEFAULT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_status_created (status, created_at),
    KEY idx_user_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
