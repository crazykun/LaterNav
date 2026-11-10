-- 分类主数据。sites.category 保留名称投影，重命名由分类服务同步更新。
CREATE TABLE IF NOT EXISTS categories (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_category_name (name),
    KEY idx_category_sort (sort_order, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 可重入回填：包含软删除站点的分类，避免恢复后出现未管理分类。
INSERT INTO categories (name, sort_order)
SELECT DISTINCT s.category, 0
FROM sites s
WHERE s.category <> ''
  AND NOT EXISTS (SELECT 1 FROM categories c WHERE c.name = s.category);

-- 兼容 MySQL 5.7：按名称和自增 ID 生成排序值，不使用窗口函数。
-- 新建分类时 sort_order = MAX(sort_order) + 10，因此 ID 即稳定的初始顺序。
UPDATE categories
SET sort_order = id * 10
WHERE sort_order = 0;
