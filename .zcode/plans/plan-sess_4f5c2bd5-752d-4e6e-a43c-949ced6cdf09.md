# 后台布局与会员管理增强计划

## Context

后台左侧导航目前跟随长页面滚出视口；会员列表一次性加载全部用户，只显示登录名，且没有搜索、分页和三方身份资料。此次调整让后台导航在长页面保持可见，并把会员列表升级为可搜索分页的资料管理视图，展示昵称及每个三方登录平台的头像、邮箱、昵称。

数据库已有 `oauth_identities.picture/email/display_name/preferred_username`，无需新增表；但现有登录流程未采集头像和邮箱，需要同步补齐，否则新增列会持续为空。

## 一、固定后台左侧导航

修改 `static/css/style.css`，保留现有 `admin-shell` DOM：
- 桌面端给 `.admin-sidebar` 使用 `position: sticky; top: 0; align-self: start`，高度为 `100vh/100dvh`，内部 `overflow-y: auto`，正文继续正常滚动。
- 使用 `border-box` 避免 padding 超出视口；低高度窗口只滚动侧栏内部。
- 窄屏改为单列、侧栏静态排列，避免当前固定 280px 侧栏挤压内容；不额外引入抽屉或 JS。
- 保持 `admin-main`、topbar 的现有行为，不把整个后台改成双滚动容器。

## 二、会员搜索与数据库分页

在 `internal/models/user.go` 增加管理端展示模型：
- `AdminUser` 嵌入 `User`，带 `OAuthIdentities []OAuthIdentity`。
- `OAuthIdentity` 包含 provider 展示名/类型/标识，以及 `PreferredUsername`、`DisplayName`、`Picture`、`Email`；提供昵称/头像展示回退方法，模板不重复业务判断。
- 提供 `DisplayName()`（用户昵称优先，空则登录名）与 `UserPage`（用户、总数、页码、每页条数、总页数）模型。

在 `internal/database/repository/user_repo.go` 新增专用查询，保留现有 `GetAllUsers` 避免影响其他调用：
- `ListUsersPage(query string, limit, offset int)`：`COUNT(*) + LIMIT/OFFSET`，固定 `id DESC` 排序。
- 搜索使用参数绑定，覆盖 `users.username/nickname/email`，并用 `EXISTS` 覆盖 `oauth_identities.email`，避免 JOIN 导致用户与 COUNT 重复。
- `ListOAuthIdentitiesByUserIDs(userIDs)`：当前页用户 ID 一次批量查询，`LEFT JOIN oauth_providers ON identity_key = provider`，按用户分组，避免 N+1；provider 配置缺失时回退显示原始标识。

在 `internal/services/user_service.go` 增加 `GetAdminUsers(query, page, pageSize)`：
- 默认/修正页码为 1，每页固定 20 条，限制异常页大小。
- 总页数计算后将过大页码收敛到最后一页，再查询当前页及三方身份。

## 三、会员列表与操作回跳

修改 `internal/handlers/admin_handlers.go`：
- `AdminUsers` / `renderAdminUsers` 读取 `q`、`page`，调用新的分页服务；错误重绘也保留搜索和页码。
- 向模板传 `total/page/pageSize/totalPages/query/listQuery` 等分页数据。
- 改角色、重置密码、改状态成功后，使用白名单重新构造 `q/page` 查询串并回跳原列表；失败重绘保留同样条件。

重做 `templates/admin/admin-users.html` 的列表区域，沿用 `admin-toolbar-row/admin-table/admin-btn-*`：
- 顶部显示筛选后的用户总数，GET 搜索框（登录名/昵称/邮箱），搜索自动回到第 1 页，可清空。
- 主资料列显示用户昵称（空则登录名）及次级登录名；账号邮箱单独展示。
- 三方登录列按身份逐条显示：平台名、头像（无头像用昵称首字/平台占位）、三方昵称、三方邮箱、三方登录名；无身份显示“未绑定”，未返回的邮箱/昵称显示“未提供”。头像使用普通远程 URL + `loading=lazy` 和加载失败回退，不代理或上传第三方内容。
- 角色、状态、注册时间和原有账号操作保留，自己账号的限制不变。
- 底部提供总数、当前页/总页数、上一页/下一页及页码链接；所有链接保留搜索关键词。
- 三个操作表单及密码弹窗统一携带白名单回跳参数，避免操作后丢失筛选页码。

## 四、补齐三方资料采集与更新

不新增数据库迁移，使用已有字段：
- `internal/handlers/oidc.go`：OIDC claims 增加 `email`、`picture`，解析后随身份返回；不做“邮箱已验证”的错误声明。
- `internal/handlers/github_oauth.go`：GitHub userinfo 解码 `email`、`avatar_url`；保持现有 `read:user` scope，私有邮箱无法取得时显示未提供，不额外扩大权限。
- `internal/handlers/oauth_registry.go`：将 OIDC/GitHub 结果统一为身份资料参数传给用户解析逻辑。
- `databaseOIDCUser` 新用户 INSERT 写入 `picture/email`；已绑定用户登录时更新对应身份的 `preferred_username/display_name/picture/email`，让旧账号在下次三方登录后补齐资料。并发冲突回退路径也同步刷新。
- 不自动覆盖用户主动编辑的 `users.nickname`；三方昵称只写入身份表。

## 五、测试与验证

更新/新增：
- `test/admin_user_actions_test.go`：昵称优先、多个三方身份的头像/邮箱/昵称、空资料回退、搜索表单、分页链接、操作后保留 q/page；同步手写 SQL stub 的分页 COUNT、LIMIT/OFFSET 和身份批量查询。
- `test/templ_test.go`：兼容新列表模型和分页字段。
- 身份资料解析/保存测试：OIDC claims 映射、GitHub `avatar_url/email` 映射、已绑定用户刷新资料，继续复用现有测试风格，不引入新依赖。
- CSS 保留现有 class，避免破坏共享 shell 测试。

验证：
1. `go build -v ./... && go vet ./... && go test -v ./...`，改动文件 `gofmt`。
2. 本机无 MySQL server，不能声称已完成真实端到端；部署机 `./scripts/restart.sh` 后验证：长后台页面滚动时侧栏固定、低高度/窄屏布局。
3. 用超过 20 个用户验证翻页和边界页；按登录名/昵称/账号邮箱/三方邮箱搜索；操作后仍停留原筛选页。
4. 分别通过 OIDC 和 GitHub 重新登录一次，确认头像、邮箱、昵称保存并展示；已有用户需重新三方登录才会补齐此前未采集的头像和邮箱。