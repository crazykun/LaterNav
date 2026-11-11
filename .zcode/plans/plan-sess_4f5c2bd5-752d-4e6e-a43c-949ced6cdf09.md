# 友情链接功能实施计划

## Context

新增独立的友情链接模块，管理员可维护友链内容，首页展示风格与现有站点卡片协调但降低视觉权重。仅在首页站点网格下方显示，不进入搜索页或公共页脚。

## 一、数据与服务

- 新增 `012_friend_links.sql`，兼容 MySQL 5.7：单条 `CREATE TABLE IF NOT EXISTS`。
- 表字段：`id/name/url/description/logo/sort_order/enabled/created_at/updated_at`，索引 `(enabled, sort_order, id)`。
- 新增 `models/friend_link.go`、`repository/friend_link_repo.go`、`services/friend_link_service.go`。
- Repository 提供后台列表、首页启用列表、详情、创建、更新、启停、删除及相邻项排序交换。
- 名称必填；URL 缺少协议时补 `https://`，仅允许有效 http/https 地址；Logo 允许空、上传生成的 `/static/img/` 路径或合法远程图片 URL；描述限制长度。
- 排序使用稳定 `(sort_order,id)`，上移/下移在事务中交换相邻项。

## 二、后台管理

- 新增 `/admin/friend-links` 列表及新增/编辑页面。
- 支持名称、URL、描述、Logo、排序值、启用状态；列表提供启停、上移/下移、编辑和删除。
- Logo 字段泛化现有上传控件的数据绑定，友情链接和站点表单都可复用现有 `/admin/upload`，不新增重复上传接口。
- 所有新增、编辑、启停、排序、删除操作使用 POST + 独立 CSRF scope；成功回列表，失败保留输入，底层错误仅记录日志，不直接暴露 SQL。
- 后台侧边栏新增“友情链接”入口，保留当前工作区已有的菜单顺序调整。
- 新增模板注册到 `BuildSharedTemplates`。

## 三、首页展示

- `HomePage` 只加载启用的友链，查询失败降级为空并记录日志，不影响站点首页。
- 新增 `partials/friend-links.html`，插入首页站点网格之后、主内容列结束之前，避免 HTMX 搜索替换友链。
- 采用低视觉权重的白色圆角表面：小链条图标、标题“友情链接”、简洁副标题；内部为紧凑响应式网格。
- 单项包含 Logo/首字回退、名称、单行描述、小外链箭头；移动端 1 列，桌面逐步扩展至 3–4 列。
- hover 仅使用浅蓝边框/背景与轻阴影，暗色样式与现有页面保持一致；外链 `target="_blank" rel="noopener noreferrer"`。
- 无友链或搜索页不渲染整个区块，不显示空态。

## 四、测试与验证

- 测试 URL/Logo 校验、启停列表过滤、排序与目标不存在处理。
- 测试后台表单 Logo 回显、所有写操作 POST + CSRF、启停/排序/删除路径。
- 测试首页友链名称/描述/URL、Logo 与首字回退、安全外链、空数据隐藏、搜索页不显示。
- 执行 `go build -v ./... && go vet ./... && go test -v ./...` 与 `git diff --check`。
- 本机无 MySQL，部署机 `./scripts/restart.sh` 后确认迁移 `012_friend_links.sql`，再验证后台新增/启停/排序及首页桌面、移动端展示。