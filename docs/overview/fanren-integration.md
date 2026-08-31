---
title: Fanren API 集成
description: 在无限画布中使用凡人 API 的图像与视频能力
---

# Fanren API 集成

无限画布作为独立服务运行，项目和素材保存在画布自己的数据目录中；凡人主站是唯一的登录、Key、订阅、灵石、渠道和异步任务账务来源。New API / QuantumNous 仍是主站的受保护项目身份，画布不与其共用数据库或运行时。

## 入口

生产入口规划为：

```text
https://fanrenapi.com/creative/
```

主站只提供跳转，不改变 `cdn.fanrenapi.com` 的基础访问。画布通过反向代理挂在 `/creative/` 路径下，部署时由代理去掉路径前缀，再转发到画布容器。

## 账号与计费

生产画布使用凡人站单点登录。用户进入画布后，浏览器通过同域主站会话刷新短期访问令牌；画布后端只向主站核验该令牌，并以稳定的 `fanren:<user_id>` 作为本地项目关联身份，不再创建第二套用户账号。

画布配置中的 Key 选择器读取当前凡人账号的 Key 列表，只展示 Key 名称、脱敏 Key、分组和状态。用户提交时只发送 `token_id`：

1. 凡人主站校验 Key 属于当前登录用户。
2. 主站复用既有 TokenAuth、订阅、境界倍率、自动路由、灵石扣费和日志链路。
3. 画布不接触、不保存、不通过 URL 传递 Key 明文。

```text
https://fanrenapi.com
```

余额和使用结果以凡人主站为准；画布本地不会再扣除自己的 `credits`。

## 图像任务

选择 Fanren 的 `gpt-image` 模型且已登录凡人站时，画布会：

1. 通过主站专用代理提交 `POST /api/creative/images/jobs`，并携带 `X-Fanren-Token-ID`；
2. 主站内部调用现有 `POST /v1/images/jobs`，按用户 Key 的真实权限和账务结算；
3. 画布轮询主站 `GET /api/creative/images/jobs/{id}`；
4. 将主站任务状态和图片地址呈现在画布节点中。

图生图会将参考图转为 `input_images`，2K/4K 尺寸沿用画布的尺寸参数。任务失败或超时不会在画布侧重复提交，Fanren 侧的计费仍由 Fanren API Key 负责。

## 视频任务

视频通过凡人主站专用代理使用 OpenAI 风格异步接口：

```text
POST /api/creative/videos
GET  /api/creative/videos/{id}
GET  /api/creative/videos/{id}/content
```

主站保存任务、计费和日志，画布只保存任务 ID 并轮询状态；完成后通过主站内容接口读取视频。

## 开源协议

本集成不复制 New API 代码到画布仓库。画布源代码及其二次修改继续遵循上游 GNU AGPL v3 和原作者署名要求；对外提供修改版时应同步提供对应源代码和许可证信息。
