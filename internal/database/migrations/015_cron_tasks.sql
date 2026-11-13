CREATE TABLE IF NOT EXISTS cron_tasks (
    task_key VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    cron_spec VARCHAR(100) NOT NULL,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    last_run_at DATETIME(3) DEFAULT NULL,
    last_error VARCHAR(1000) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO cron_tasks (task_key, name, cron_spec, enabled)
VALUES ('benchlm_leaderboard', 'BenchLM 模型榜单同步', '0 3 * * *', 1);
