-- 导航配置表（首页左侧分类导航）
CREATE TABLE IF NOT EXISTS nav_items (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    url VARCHAR(500) NOT NULL,
    icon VARCHAR(32) NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_enabled_sort (enabled, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 默认导航项（对应原首页固定导航）
INSERT INTO nav_items (name, url, icon, sort_order) VALUES
    ('全部', '/', 'grid', 1),
    ('对话', '/search?category=AI对话', 'chat', 2),
    ('创作', '/search?category=AI创作', 'create', 3),
    ('编程', '/search?category=AI编程', 'code', 4),
    ('工具', '/search?category=AI工具', 'tool', 5);
