<div align="center">

# 🚀 AI Later

![AI Later Logo](static/img/logo.png)

**一个现代化的 AI 工具导航站**  
基于 Go + Gin + MySQL 构建，支持用户登录、收藏管理、后台运营

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go)](https://golang.org/)
[![Gin Framework](https://img.shields.io/badge/Gin-Framework-00ADD8?style=for-the-badge&logo=go)](https://gin-gonic.com/)
[![MySQL](https://img.shields.io/badge/MySQL-8.0+-4479A1?style=for-the-badge&logo=mysql&logoColor=white)](https://www.mysql.com/)
[![License](https://img.shields.io/badge/License-MIT-green.svg?style=for-the-badge)](LICENSE)

[功能特性](#-功能特性) • [快速开始](#-快速开始) • [技术栈](#-技术栈) • [项目结构](#-项目结构) • [贡献指南](#-贡献)

</div>

---

## ✨ 功能特性

<table>
<tr>
<td width="50%">

### 🔍 核心功能
- **智能搜索**: AI 工具搜索与分类筛选
- **用户系统**: 注册/登录/退出，JWT 鉴权
- **收藏管理**: 个人收藏夹与快速访问
- **访问统计**: 站点访问量与用户行为分析
- **模型排行榜**: 独立的 [BenchLM.ai](https://benchlm.ai/data) 综合榜单页面 `/leaderboard`，每日后台同步并显示数据时间与证据状态；迁移时默认添加到「导航管理」，在首页桌面和移动端顶部导航展示，可在后台调整或关闭

</td>
<td width="50%">

### 🛠️ 管理功能
- **后台管理**: 站点内容管理与审核
- **标签系统**: 灵活的分类标签体系
- **数据持久化**: MySQL 可靠存储
- **轻量交互**: HTMX + Alpine.js 无刷新体验

</td>
</tr>
</table>

## 🛠️ 技术栈


| 类别 | 技术方案 | 说明 |
|:---:|:---|:---|
| **后端框架** | Go 1.24 + Gin | 高性能 Web 框架 |
| **模板引擎** | Go HTML Templates | 服务端渲染 |
| **前端样式** | Tailwind CSS | 原子化 CSS 框架 |
| **前端交互** | HTMX + Alpine.js | 轻量级无刷新体验 |
| **数据库** | MySQL 8.0+ | 关系型数据库 |
| **身份认证** | JWT | 无状态鉴权方案 |
| **配置管理** | YAML | 灵活的配置文件 |


## 🚀 快速开始

### 📋 环境要求

- **Go**: 1.24 或更高版本
- **MySQL**: 8.0 或更高版本
- **Git**: 用于克隆项目

### 📦 安装步骤

```bash
# 1️⃣ 克隆项目
git clone https://github.com/crazykun/ai-later.git
cd ai-later

# 2️⃣ 安装依赖
go mod download

# 3️⃣ 配置文件
cp config.demo.yaml config.yaml
# 编辑 config.yaml，填入 MySQL 连接信息和 JWT secret

# 4️⃣ 初始化数据库
go run scripts/create_db/main.go

# 5️⃣ 启动服务
go run main.go
```

### 🎉 访问应用

打开浏览器访问: **http://localhost:8080**

> 💡 **提示**: 首次启动会自动执行数据库迁移，创建必要的表结构。
>
> 排行榜首次无数据时显示空状态，不会在启动时访问上游；数据完全由后台任务管理。由 `robfig/cron/v3` 按 `cron_tasks` 表配置每天 UTC 03:00 执行（可在后台「定时任务」启停或调整为每天其他固定时间）。自动任务同一 UTC 日期成功同步后不会重复拉取；多实例部署时各实例每分钟读取任务表同步调度配置。后台「定时任务」提供独立的「立即执行」按钮：管理员可在计划任务关闭或当天已同步时异步发起手动补偿，刷新页面查看结果，不会更改计划配置。自动和手动同步失败后均按 1、2、4、8、16 秒间隔重试 5 次；全部失败时保留旧榜单，并记录最近错误。页面访问不会实时请求 BenchLM，新安装首次同步完成前显示空状态。部署时须随二进制提供 `internal/database/migrations/`，迁移文件目前从工作目录读取。

## ⚙️ 配置说明

配置文件 `config.yaml` 示例：

```yaml
# 服务端口
port: 8080

# MySQL 数据库配置
mysql:
  host: localhost
  port: 3306
  username: root
  password: "your-password"
  database: ai_later

# JWT 认证配置
jwt:
  secret: your-jwt-secret-change-me  # ⚠️ 生产环境请务必修改
  expire_days: 7                      # Token 有效期（天）
```

> ⚠️ **安全提示**: 生产环境请使用强密码和复杂的 JWT secret，不要使用默认值。

## 🗄️ 数据库

### 数据库迁移

Migration SQL 文件位于 `internal/database/migrations/`，应用启动时会自动执行。

### 核心数据表

| 表名 | 说明 |
|:---|:---|
| `sites` | AI 工具站点信息 |
| `tags` | 分类标签 |
| `site_tags` | 站点与标签关联表 |
| `users` | 用户账户信息 |
| `favorites` | 用户收藏记录 |
| `visits` | 站点访问统计 |
| `leaderboard_sync` | BenchLM 综合榜单当前快照及同步时间 |
| `leaderboard_models` | 每次成功同步发布的独立模型榜单数据 |
| `cron_tasks` | 内置定时任务的计划、启停状态和运行结果 |

排行榜数据来自 [BenchLM.ai](https://benchlm.ai/data)，标记为 [CC BY-NC 4.0](https://creativecommons.org/licenses/by-nc/4.0/)；本功能只供非商业用途，需保留页面来源链接和署名。若网站改变用途，请重新核查 [BenchLM 条款](https://benchlm.ai/terms)及上游测量数据权利。

## 📁 项目结构

```
ai-later/
├── 📄 main.go                      # 应用入口
├── 📄 config.demo.yaml             # 配置模板
├── 📄 go.mod                       # Go 模块依赖
│
├── 📂 internal/                    # 内部包（不对外暴露）
│   ├── 📂 config/                  # 配置加载与解析
│   ├── 📂 database/
│   │   ├── 📂 migrations/          # SQL 迁移文件
│   │   └── 📂 repository/          # 数据访问层（DAO）
│   ├── 📂 handlers/                # HTTP 请求处理器
│   ├── 📂 middleware/              # 中间件（JWT 鉴权、日志等）
│   ├── 📂 models/                  # 数据模型定义
│   ├── 📂 services/                # 业务逻辑层
│   └── 📂 utils/                   # 工具函数
│
├── 📂 scripts/                     # 脚本工具
│   ├── 📂 create_db/               # 数据库初始化脚本
│   └── 📄 init_mysql.sh            # MySQL 初始化脚本
│
├── 📂 static/                      # 静态资源
│   ├── 📂 css/                     # 样式文件
│   ├── 📂 js/                      # JavaScript 文件
│   └── 📂 img/                     # 图片资源
│
└── 📂 templates/                   # HTML 模板
    ├── 📄 index.html               # 首页
    ├── 📄 login.html               # 登录页
    └── ...                         # 其他页面模板
```

## 🤝 贡献

欢迎贡献代码！请遵循以下步骤：

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 提交 Pull Request

### 代码规范

- 遵循 Go 官方代码风格
- 使用 `gofmt` 格式化代码
- 添加必要的注释和文档
- 确保所有测试通过

## 📝 开发路线图

- [x] 基础功能实现
- [x] 用户认证系统
- [x] 收藏与统计功能
- [ ] 搜索优化（全文搜索）
- [ ] 标签推荐算法
- [ ] API 接口开放
- [ ] Docker 容器化部署
- [ ] 单元测试覆盖

## 🐛 问题反馈

如果您发现任何问题或有功能建议，请：

- 提交 [Issue](https://github.com/crazykun/ai-later/issues)
- 或通过 [Pull Request](https://github.com/crazykun/ai-later/pulls) 直接贡献代码

## 📄 License

本项目采用 [MIT License](LICENSE) 开源协议。

---

<div align="center">

**[⬆ 回到顶部](#-ai-later)**

Made with ❤️ by [crazykun](https://github.com/crazykun)

如果这个项目对你有帮助，请给个 ⭐️ Star 支持一下！

</div>
