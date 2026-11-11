-- 分类图标：复用导航图标体系（grid/chat/create/code/tool/star/link），空值按名称自动匹配。
ALTER TABLE categories
    ADD COLUMN icon VARCHAR(32) NOT NULL DEFAULT '' AFTER name;
