CREATE TABLE IF NOT EXISTS leaderboard_sync (
    id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
    current_snapshot BIGINT UNSIGNED DEFAULT NULL,
    generated_at DATETIME(3) DEFAULT NULL,
    synced_at DATETIME(3) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO leaderboard_sync (id) VALUES (1);

CREATE TABLE IF NOT EXISTS leaderboard_models (
    snapshot_id BIGINT UNSIGNED NOT NULL,
    model_key VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    name VARCHAR(255) NOT NULL,
    creator VARCHAR(255) NOT NULL DEFAULT '',
    overall_rank INT UNSIGNED NOT NULL,
    score DECIMAL(6,2) NOT NULL,
    evidence_status VARCHAR(32) NOT NULL,
    source_url VARCHAR(500) NOT NULL,
    PRIMARY KEY (snapshot_id, model_key),
    KEY idx_snapshot_rank (snapshot_id, overall_rank)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO nav_items (name, url, description, icon, sort_order, enabled)
SELECT '模型排行', '/leaderboard', 'BenchLM 综合模型榜单', 'star', 6, 1
WHERE NOT EXISTS (SELECT 1 FROM nav_items WHERE url = '/leaderboard');
