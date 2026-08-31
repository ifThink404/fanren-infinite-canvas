<p align="center">
  <img src="web/public/logo.svg" width="96" alt="infinite-canvas logo">
</p>

<h1 align="center">无限画布 (infinite-canvas)</h1>

<p align="center">
  <a href="https://github.com/tigerowo/infinite-canvas"><img src="https://img.shields.io/github/stars/tigerowo/infinite-canvas?style=flat-square&logo=github" alt="GitHub stars"></a>
  <a href="VERSION"><img src="https://img.shields.io/badge/version-v0.6.0-2563eb?style=flat-square" alt="Version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-f97316?style=flat-square" alt="License"></a>
  <a href="https://www.docker.com/"><img src="https://img.shields.io/badge/Docker-ready-2496ed?style=flat-square&logo=docker&logoColor=white" alt="Docker ready"></a>
  <a href="https://nextjs.org/"><img src="https://img.shields.io/badge/Next.js-16.2-000000?style=flat-square&logo=nextdotjs" alt="Next.js"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.25-00add8?style=flat-square&logo=go&logoColor=white" alt="Go"></a>
</p>

<p align="center">
  <a href="#联系方式"><img src="https://img.shields.io/badge/微信交流群已开放-扫码加入-07C160?style=flat-square&logo=wechat&logoColor=white" alt="微信群"></a>
</p>

<p align="center">
  <strong>Windows 本地安装包现已同步更新</strong><br>
  <sub>无需从源码构建，下载即可本地运行</sub>
</p>

<p align="center">
  <a href="https://github.com/tigerowo/infinite-canvas/releases/latest">
    <img
      src="https://img.shields.io/github/v/release/tigerowo/infinite-canvas?style=for-the-badge&logo=windows11&logoColor=white&label=Windows%20EXE&color=2563eb"
      alt="下载 Windows EXE"
    >
  </a>
</p>

无限画布是一款面向图片，视频，音频，全能创作的开源工作台。它把画布编排、AI 图片、视频、音频生成、参考图编辑、对话助手、提示词库和素材沉淀放在同一个界面里，适合用来探索视觉方案并连续迭代图片结果。

本仓库同时维护凡人站集成版本：画布仍然独立运行，但生产环境使用凡人站作为统一的登录、Key、灵石、订阅、境界倍率、自动路由、日志和异步任务账务中心。集成设计与接口约定见 [Fanren API 集成说明](docs/overview/fanren-integration.md)。

## 凡人 API 集成

### 生产入口

生产用户从凡人主站进入无限画布：

```text
https://fanrenapi.com/creative/
```

这是同一站点下的独立路径，不需要创建第二个画布账号。凡人主站的普通页面和 `cdn.fanrenapi.com` 的基础访问不依赖画布路由，画布故障时不会改变主站 API 的路由行为。

### 账号、Key 与灵石

凡人站是身份和账务的唯一来源：

1. 用户在凡人主站登录，画布登录页跳转到主站登录并携带回跳地址。
2. 画布后端用主站会话令牌向 `GET /api/user/self` 校验身份，不接受独立画布 JWT 作为生产用户身份。
3. 画布配置弹窗从主站读取当前用户的 Key 列表，选择器展示 Key 名称、脱敏 Key、分组和状态；提交时只发送 `token_id`，不让用户手填 Key，也不把 Key 明文放进 URL。
4. 主站根据用户当前订阅、灵蕴身份、境界倍率、分组路由和可用渠道完成扣费；画布不维护第二套余额，不在本地重复扣除灵石。
5. 使用日志、异步任务归属和账单审计均以凡人主站记录为准。

画布端发送 Key 选择头：

```http
Authorization: Bearer <凡人站会话令牌>
X-Fanren-Token-ID: <当前用户拥有的token_id>
```

主站会再次校验 `token_id` 必须属于当前用户，然后在内部复用原有的 TokenAuth、订阅和计费链路。任何客户端都不应绕过这个校验直接提交其他用户的 Key。

### 主站代理接口

浏览器访问画布时，下面的请求由画布同源代理到凡人主站。主站再转入现有 `/v1` 转发链路：

| 用途 | 方法与路径 |
| --- | --- |
| 图片异步提交 | `POST /api/creative/images/jobs` |
| 查询图片任务 | `GET /api/creative/images/jobs/{id}` |
| 图片同步生成 | `POST /api/creative/images/generations` |
| 图片编辑 | `POST /api/creative/images/edits` |
| 视频异步提交 | `POST /api/creative/videos` |
| 查询视频任务 | `GET /api/creative/videos/{id}` |
| 读取已完成视频 | `GET /api/creative/videos/{id}/content` |
| Responses 对话 | `POST /api/creative/responses` |
| Chat Completions 对话 | `POST /api/creative/chat/completions` |
| 语音生成 | `POST /api/creative/audio/speech` |

图片和视频任务均采用异步模式：提交接口只负责创建任务并返回任务 ID，画布按间隔轮询状态；完成后再读取结果。浏览器刷新不会重新提交任务，也不会因为轮询超时重复扣费。

图片请求支持文生图、图生图、`1K`/`2K`/`4K` 尺寸和批量任务，具体可用能力由当前 Key、主站渠道和上游模型共同决定。画布会把参考图转换为主站代理约定的 `input_images` 字段；不要把上游地址或上游 Key 配置在画布前端。

### 部署拓扑

当前生产链路如下：

```text
浏览器
  -> https://fanrenapi.com/creative/
  -> DMIT Nginx: /creative/ 独立反向代理
  -> DMIT 127.0.0.1:23011
  -> SSH 隧道
  -> fr-netcup-new 127.0.0.1:13011
  -> fanren-infinite-canvas 容器
```

凡人主站的 `/api/creative/*` 仍由主站处理；`cdn.fanrenapi.com` 不指向画布。Nginx 同时配置了 `/creative` 根路径和 `/creative/` 子路径，避免 Next.js 根路由重定向循环。

### 配置项

生产集成至少需要以下配置：

```dotenv
FANREN_AUTH_BASE_URL=https://fanrenapi.com
FANREN_SSO_REQUIRED=true
PUBLIC_BASE_URL=https://fanrenapi.com/creative
NEXT_PUBLIC_BASE_PATH=/creative
NEXT_PUBLIC_FANREN_SSO=true
FANREN_IMAGE_JOB_POLL_SECONDS=5
FANREN_IMAGE_JOB_TIMEOUT_SECONDS=1800
```

本地独立开发可以将 `FANREN_SSO_REQUIRED=false`、`NEXT_PUBLIC_FANREN_SSO=false`，使用画布自己的本地登录和兼容 OpenAI 的 Base URL；生产环境必须保持 SSO 开启。完整变量示例见 [.env.example](.env.example)。

### 构建、部署与回滚

构建机直接构建镜像并传输到 `fr-netcup-new`，不会通过本机运行中的容器提供生产流量：

```bash
./scripts/deploy-fanren-canvas.sh
```

脚本会检查工作树、在构建机生成镜像、更新 Netcup 画布容器、确认健康检查、建立 DMIT 持久 SSH 隧道并验证凡人主站、CDN 和画布入口。管理员密码和 JWT 密钥复用目标机已有 `.env`；首次部署时脚本只在终端显示一次生成的初始密码，敏感配置不会写入 Git。

每次部署会在网关的 restore 目录保存 Nginx 配置备份。若新版本健康检查失败，先停止继续切流，使用对应备份恢复 Nginx，再在 Netcup 使用上一镜像启动容器。主站版本发布仍使用主站自己的蓝绿脚本；画布仓库只负责画布镜像和 `/creative/` 路由。

### 本地联调

本地需要同时能访问凡人主站：

```bash
cp .env.example .env
# .env 中开启 FANREN_AUTH_BASE_URL 和 FANREN_SSO_REQUIRED
go run .

cd web
bun install
NEXT_PUBLIC_FANREN_SSO=true NEXT_PUBLIC_BASE_PATH=/creative bun run dev
```

联调验收顺序：先确认 `/creative/login` 能跳转凡人登录；登录后确认 Key 下拉列表只出现当前账号的 Key；再用一个低成本模型验证提交、轮询、失败展示和日志归属。没有有效的凡人会话和 Key 时，不要用伪造的 `token_id` 代替真实扣费测试。

## 赞助商

<table>
  <tr>
    <td width="190" align="center">
      <a href="https://metaso.cn/minimax-h3/?s=tt" target="_blank" rel="noopener"><img src="assets/metaso.png" width="163" alt="秘塔科技"></a>
    </td>
    <td>
      <strong>MiniMax H3 视频生成 API｜秘塔科技</strong> 秘塔科技提供高性价比的 MiniMax H3 视频生成服务：<strong>768P 仅 0.09 元/秒，2K 仅 0.15 元/秒</strong>。支持原生 2K、音画同步，API 兼容 <strong>OpenAI 协议</strong>，同时支持 <strong>ComfyUI</strong>，无需自行部署 GPU。 🎁 通过 <a href="https://metaso.cn/minimax-h3/?s=tt" target="_blank" rel="noopener noreferrer">无限画布专属链接注册</a>，即可领取赠送额度及专属优惠。
    </td>
  </tr>
</table>

<p align="center">
  <a href="https://metaso.cn/minimax-h3/?s=tt" target="_blank" rel="noopener"><img src="https://raw.githubusercontent.com/tigerowo/cdn-tdeh/v0.6/img/infinite-canvas/metaso.webp" alt="3D 导演台时间轴" /></a>
</p>
<p align="center">
  <img src="https://raw.githubusercontent.com/tigerowo/cdn-tdeh/v0.5/img/infinite-canvas/3ddirectortl.webp" alt="3D 导演台时间轴" />
</p>
<p align="center">
  <img src="https://raw.githubusercontent.com/tigerowo/cdn-tdeh/v0.4/img/infinite-canvas/3ddirector.webp" alt="3D 导演台" />
</p>
<p align="center">
  <img src="https://gcore.jsdelivr.net/gh/tigerowo/cdn-tdeh@v0.4/img/infinite-canvas/agent.webp" alt="Agent" />
</p>
<p align="center">
  <img src="https://raw.githubusercontent.com/tigerowo/cdn-tdeh/v0.4/img/infinite-canvas/panorama.webp" alt="全景图生成" />
</p>

本项目基于 [basketikun(纯前端)](https://github.com/basketikun/infinite-canvas) 为底，合并 [HuFakai](https://github.com/HuFakai/infinite-canvas) 生图增强版基础上，针对视频和视频生成逻辑配置更加完善，完善后端云同步机制，不再依赖纯前端

> [!CAUTION]
> 项目目前处于开发阶段，不保证历史数据兼容。各种数据库结构和存储格式都可能直接调整，欢迎关注后续更新
>
> 如果你需要稳定维护自己的分支，建议自行 fork 后独立开发。二次开发与 PR 请保留原作者信息和前端页面标识

## 核心功能

- 全景图：支持文字生成、参考图生成和本地 2:1 全景图导入，可作为导演台的场景环境背景
- 导演台：在独立 3D 场景中布置角色、模型、全景环境和机位，支持镜头管理、截图，并将机位画面自动发送为连线图片节点
- 摄像机控制：图片、视频和生成配置节点支持独立设置相机、镜头、焦距和光圈，将镜头参数自动写入生成提示词，并随节点保存和复制
- 无限画布：多画布项目、节点拖拽缩放、连线、小地图、撤销重做、导入导出
- AI 创作：支持 OpenAI 兼容接口的 Images API、Responses API、图生图、参考图编辑、流式接收、Base64 图片返回；Seedance 2.0 可通过火山方舟 Agent Plan 接入
- 生图工作台：支持侧边/悬浮底部工作台、多任务并发、历史结果合并展示、分类管理、失败详情、参考图缩略图、图片体积展示和“我的素材”复用
- 创作工作流：支持公开/个人模板、变量表单、AI 创建工作流、单图/多图系列工作流、参考图输入和结果自动进入生图历史
- 画布助手：围绕选中节点和上游节点对话、生图，并把结果插回画布
- 提示词库：抓取多个 GitHub 开源项目，按案例整理数百个图片提示词
- 提示词与素材：提示词库、服务器素材库和“我的素材”可在生图、画布 AI 和工作流中复用

完整功能说明见 [docs/features.md](docs/overview/features.md)

如果你在为担心没有合适的生图API来发愁，可以查看该免费生图项目：[chatgpt2api](https://github.com/basketikun/chatgpt2api)

## 技术栈

- 前端：Next.js、React、TypeScript、Tailwind CSS、Ant Design、Zustand、TanStack Query
- 后端：Go、Gin、GORM
- 存储：SQLite、本地 IndexedDB、S3 兼容对象存储、Cloudflare R2  
- 部署：Docker

## 快速开始

```bash
git clone https://github.com/ifThink404/fanren-infinite-canvas.git
cd fanren-infinite-canvas
cp .env.example .env
# 修改默认账号密码等信息
docker compose up -d --build
```

本地非 Docker 开发运行：
```bash
cp .env.example .env
go run .

# 另开一个终端窗口
cd web
bun install
bun run dev
```

本地源码构建运行：

```bash
cp .env.example .env
docker compose -f docker-compose.local.yml up -d --build
```

运行后默认端口3000，可访问 `http://localhost:3000`

如需要拉取提示词，可前往:`http://localhost:3000/admin/prompts`

## New API 兼容模式

本项目保留上游 New API 的手动配置能力，适合本地独立开发或连接其他 OpenAI 兼容网关。可在 `系统设置 -> 聊天方式 -> 添加聊天设置` 中填入：

```text
https://infinite-canvas-cpco.onrender.com?apiKey={key}&baseUrl={address}
```

跳转后会自动打开配置弹窗并填入 API Key 和 Base URL。如果自己部署了，可以把 `https://infinite-canvas-cpco.onrender.com` 替换成你部署的地址。

这条 `apiKey` 查询参数方式只用于兼容模式，不属于凡人生产集成。凡人生产入口使用同源 SSO 和配置弹窗中的 Key 下拉选择，不把 Key 放进 URL；不要把真实 Key 写入 README、Issue、截图、构建参数或 Git 历史。New API / QuantumNous 仍是上游与主站的受保护项目身份，二次开发请保留原作者信息和许可证要求。

## 效果展示

<table width="100%">
  <tr>
    <td width="50%"><img src="https://cdn3.ldstatic.com/original/4X/d/7/c/d7cecc7df20fcd935ce760757f8799cf4436c936.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://cdn3.ldstatic.com/original/4X/6/0/7/607af375f9182a86f31655b8326337a536f70e34.png" alt="image" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://cdn3.ldstatic.com/original/4X/6/e/6/6e60f82eec3602151abccc60fc4b55d028ac8415.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://cdn3.ldstatic.com/original/4X/8/b/a/8bae005a727727c8d83e0e01b05fea90155e56a5.jpeg" alt="image" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://cdn3.ldstatic.com/original/4X/e/b/e/ebe20a7cb4c4837495cdbd55b4327fa741ce2938.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://cdn3.ldstatic.com/original/4X/0/f/b/0fbe4f543ac554a7950cf011ceb4586d27e6d681.png" alt="image" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://i.ibb.co/MxXZkWc7/1.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/5g46rH3L/2.png" alt="image" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://i.ibb.co/NfHpv5q/3.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/svXg7dPp/4.png" alt="image" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://cdn3.ldstatic.com/original/4X/8/6/7/867532c5c6dfff38cfa2b90ca0e0f76809b066d4.png" alt="5" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/BHjjXcV4/6.png" alt="image" border="0"></td>
  </tr>
</table>

## 文档

- [功能介绍](docs/overview/features.md)
- [部署说明](docs/overview/docker.md)
- [画布节点操作手册](docs/canvas/canvas-node-manual.md)
- [画布快捷键](docs/canvas/canvas-shortcuts.md)
- [待办事项](docs/progress/todo.md)
- [后端数据库说明](docs/backend/backend-database.md)
- [系统配置数据结构](docs/backend/system-settings.md)
- [接口响应约定](docs/backend/api-response.md)

## 联系方式

项目定制二次开发需求，广告赞助合作其他可联系

邮箱：yhb293933@gmail.com

微信交流测试群：
<p align="center">
  <img src="assets/wc.png" alt="微信群二维码" width="180">
</p>

## 赞助支持

<div align="center">

本项目长期开放广告赞助合作，欢迎品牌 / 产品投放

如果这个项目对你有帮助，欢迎赞助支持，你的每一份鼓励都是持续更新的动力！

</div>

## 社区支持

学 AI，上 L 站：[LinuxDO](https://linux.do/)

## 开源协议

本项目使用 GNU Affero General Public License v3.0，见 [LICENSE](LICENSE)。

## Star History

<a href="https://www.star-history.com/?repos=tigerowo%2Finfinite-canvas&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=tigerowo/infinite-canvas&type=date&theme=dark&legend=top-left&sealed_token=SMYnxdZ99ogoiNPY5Qaeg1X9nB17KGpOCvv0Pzjz5mLCx5o7pNOpQNnYpk2CIUkdJMuAcxve8H_ZAYllKY4b7YTvZh0tiHoC8hGfknKnk2IUMYhQoIxgcQ" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=tigerowo/infinite-canvas&type=date&legend=top-left&sealed_token=SMYnxdZ99ogoiNPY5Qaeg1X9nB17KGpOCvv0Pzjz5mLCx5o7pNOpQNnYpk2CIUkdJMuAcxve8H_ZAYllKY4b7YTvZh0tiHoC8hGfknKnk2IUMYhQoIxgcQ" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=tigerowo/infinite-canvas&type=date&legend=top-left&sealed_token=SMYnxdZ99ogoiNPY5Qaeg1X9nB17KGpOCvv0Pzjz5mLCx5o7pNOpQNnYpk2CIUkdJMuAcxve8H_ZAYllKY4b7YTvZh0tiHoC8hGfknKnk2IUMYhQoIxgcQ" />
 </picture>
</a>
