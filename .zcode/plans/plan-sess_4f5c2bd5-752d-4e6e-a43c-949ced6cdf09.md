# 后台分类管理与标签管理规划

## Context

当前分类只是 `sites.category` 字符串，首页桌面侧栏和移动胶囊还分别硬编码分类；标签已有 `tags + site_tags` 表，但没有管理入口，历史编辑还会遗留孤儿标签。本次补齐两个后台模块，并让分类真正成为可维护的主数据。

已确认规则：
- **分类受控选择**：后台添加/编辑站点、会员投稿只能选择已有分类，不能任意创建；仍允许“未分类”。
- **使用中分类禁止删除**：先把站点迁到其他分类再删除，避免误清空。
- **标签可删除、可合并**：删除即解除关联；合并后关联去重，源标签删除。

## 一、数据库与数据模型

新增 `internal/database/migrations/011_categories.sql`：
- 建 `categories(id, name UNIQUE, sort_order, created_at, updated_at)`，分类名最长 100 字符。
- 从现有全部 `sites.category` 去重回填（包含软删除站点，避免恢复后产生未管理分类）。
- 迁移 SQL 采用可重复执行的 `INSERT ... SELECT ... WHERE NOT EXISTS`，适配现有非事务 migration runner。
- 保留 `sites.category` 字符串作为现有查询/展示路径，**本次不迁移为 `category_id`**，避免双写、外键迁移和回滚复杂性；由 service 保证分类主数据与站点名称一致。

新增模型：
- `internal/models/category.go`：`Category`、`CategoryUsage{Category, SiteCount}`。
- `internal/models/tag.go`：`Tag`、`TagUsage{Tag, SiteCount}`。
- 使用数只统计 `deleted=0` 的正式站点，界面明确“使用数不含已删除站点和投稿快照”。

## 二、分类管理

新增 `category_repo.go` / `category_service.go`：
- 列表：支持名称搜索、数据库分页、按 `sort_order + id` 排序，LEFT JOIN/子查询统计站点数。
- 新增：Trim、非空、长度校验、重名返回业务错误；允许零站点分类。
- 排序：列表提供上移/下移，事务内交换相邻两条 `sort_order`；初始化使用稳定递增排序值。
- 重命名：事务内更新 `categories`、全部 `sites.category`、待审核 `site_submissions.category`；已通过/已驳回投稿保留历史快照。
- 删除：使用数大于 0 返回“请先迁移此分类下的站点”，仅允许删除未使用分类；空分类可直接删除。
- 提供 `ListEnabled`/`ExistsByName` 给前台和表单复用；分类名称仍是现有搜索 URL 的参数，不引入 slug 破坏现有链接。

## 三、标签管理

新增 `tag_repo.go` / `tag_service.go`：
- 列表：名称搜索、分页、正式站点使用数，按名称/使用数排序。
- 新增与重命名：复用统一标签规范化规则，处理空名、超长、重名。
- 删除：事务中删除 `site_tags` 关联与标签，后台显示影响站点数并要求确认。
- 合并：源标签 → 目标标签，事务内执行：
  1. 校验两个标签存在且不是同一个；
  2. `INSERT IGNORE INTO site_tags ... SELECT ...` 合并关联；
  3. 删除源关联和源标签；
  4. 对待审核投稿的标签字符串按完整项替换、去重后重写，不能 SQL 子串替换。
- 已通过/已驳回投稿保留历史标签快照。
- 顺带修复现有编辑站点“清空标签后保存不会解除旧关联”的问题：空输入传递空切片而不是 `nil`；新增/编辑统一支持中英文逗号、去空去重。

## 四、后台路由、页面与交互

新增 `internal/handlers/admin_taxonomy_handlers.go`，在 `main.go` 的管理员组注册：
```text
GET  /admin/categories
POST /admin/categories/add
POST /admin/categories/:id/rename
POST /admin/categories/:id/move
POST /admin/categories/:id/delete

GET  /admin/tags
POST /admin/tags/add
POST /admin/tags/:id/rename
POST /admin/tags/:id/merge
POST /admin/tags/:id/delete
```

页面：
- 新增 `templates/admin/admin-categories.html`：搜索、使用数、排序、添加、重命名、删除；使用中分类的删除按钮禁用并说明原因。
- 新增 `templates/admin/admin-tags.html`：搜索、使用数、添加、重命名、合并目标选择、删除确认。
- 两页复用后台 shell、表格、flash、分页；操作后保留 `q/page`。
- 泛化当前用户专用分页 URL helper/分页 partial，支持传入基础路径，不复制三套分页逻辑。
- 新增加改名、删除、合并、排序 POST 的后台 CSRF 校验，不继续沿用旧导航的 GET 删除模式。
- `admin-shell.html` 在“站点管理”附近新增“分类管理”“标签管理”入口。
- `internal/web/templates.go` 注册新模板，避免运行时模板缺失。

## 五、打通分类与现有业务

- **首页 `templates/index.html`**：桌面侧栏和移动胶囊共用同一份分类列表，移除硬编码的“AI对话/AI创作/AI编程/AI工具”；分类项统一 `/search?category=<名称>`，保留“全部”、当前分类高亮及现有悬浮/移动布局。分类图标用通用图标，不再错误绑定分类名。
- **后台站点新增/编辑**：分类文本框改为 `select`，选项来自分类 service，提供“未分类”；旧站点若存在未管理分类，迁移后应有对应选项。
- **会员投稿**：分类改为已有分类下拉选择，允许不选；`SiteSubmissionService.Submit` 服务端校验分类存在，不能只做前端限制。
- **审核通过**：再次校验投稿分类仍有效；若分类已被删除，提示管理员先处理投稿，而不是重新产生旧分类。
- **导航管理 `nav_items`**：顶部自由链接与分类继续独立；分类改名只同步明确匹配 `/search?category=<旧名>` 的导航 URL，避免影响其他自定义链接。
- **仪表盘统计**：分类数量改读主数据表；分类统计保留“正式站点使用数”口径。标签总数继续读取 `tags`。
- `GetCategories()` 保留兼容包装，逐步委托分类模块，避免一次性改坏现有 handler 和 SQL stub。

## 六、测试与验证

新增/更新测试：
- 分类新增、重名、重命名级联、使用中删除保护、排序事务、搜索分页。
- 标签重命名、删除关联、合并时同站重复关联去重、源标签删除、事务回滚。
- 待审核投稿同步与历史投稿快照不变。
- 站点编辑空标签可清空，标签规范化一致。
- 首页桌面/移动动态分类、URL 转义、选中态、空分类不报错。
- 后台模板、CSRF、操作后 `q/page` 回跳；更新 `test/nav_test.go` 的硬编码分类断言。

验证：
1. `go build -v ./... && go vet ./... && go test -v ./...`，改动文件 `gofmt`。
2. 本机无 MySQL，真实迁移和端到端在部署机执行 `./scripts/restart.sh` 后验证。
3. 核对迁移回填分类数与 `SELECT DISTINCT category FROM sites` 一致。
4. 实测：新增空分类 → 首页/站点/投稿同步出现 → 新建分类站点 → 重命名后前台链接和站点仍正常 → 使用中分类禁止删除 → 清空分类后删除。
5. 实测标签删除、两个标签合并、同一站点关联不重复，以及投稿审核不会产生已合并旧标签。

## 范围控制

本次不引入 `sites.category_id` 大迁移、不做标签前台筛选页、不重做审核原子性。分类管理先以“主数据表 + 受控写入 + 名称同步”落地，避免把两个后台管理功能扩大成数据模型重构。