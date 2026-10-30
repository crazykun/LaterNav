-- 导航项新增描述字段（悬停提示文案）
ALTER TABLE nav_items ADD COLUMN description VARCHAR(255) NOT NULL DEFAULT '' AFTER url;

-- 为已上线的默认导航补充描述
UPDATE nav_items SET description = '查看全部站点' WHERE name = '全部' AND description = '';
UPDATE nav_items SET description = 'AI 对话类工具' WHERE name = '对话' AND description = '';
UPDATE nav_items SET description = 'AI 图片、视频、音频创作工具' WHERE name = '创作' AND description = '';
UPDATE nav_items SET description = 'AI 编程与开发助手' WHERE name = '编程' AND description = '';
UPDATE nav_items SET description = '效率与办公 AI 工具' WHERE name = '工具' AND description = '';
