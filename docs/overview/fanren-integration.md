---
title: Fanren API 集成
description: 在无限画布中使用凡人 API 的图像与视频能力
---

# Fanren API 集成

无限画布作为独立服务运行，项目、素材和异步任务保存在画布自己的数据目录中。New API / QuantumNous 只作为上游业务接口，不与画布共用数据库或运行时。

## 入口

生产入口规划为：

```text
https://fanrenapi.com/creative/
```

主站只提供跳转，不改变 `cdn.fanrenapi.com` 的基础访问。画布通过反向代理挂在 `/creative/` 路径下，部署时由代理去掉路径前缀，再转发到画布容器。

## 账号与计费

画布账号与 Fanren 账号是两个独立的登录域。要让请求按 Fanren 用户的分组、订阅和灵石规则结算，请在画布的渠道配置中使用该用户自己的 Fanren API Key，Base URL 填：

```text
https://fanrenapi.com
```

不要把 API Key 放在跳转 URL、查询参数、日志或公开渠道配置中。当前入口只预填 Base URL，不携带密钥；用户首次使用时在画布配置中填写密钥。

## 图像任务

选择 Fanren 的 `gpt-image` 模型且已登录画布时，画布会：

1. 创建本地持久化任务；
2. 调用 Fanren `POST /v1/images/jobs`；
3. 在服务端轮询 `GET /v1/images/jobs/{id}`；
4. 将完成状态和图片地址写回画布任务记录。

图生图会将参考图转为 `input_images`，2K/4K 尺寸沿用画布的尺寸参数。任务失败或超时不会在画布侧重复提交，Fanren 侧的计费仍由 Fanren API Key 负责。

## 视频任务

视频使用 Fanren 的 OpenAI 风格异步接口：

```text
POST /v1/videos
GET  /v1/videos/{id}
GET  /v1/videos/{id}/content
```

画布保存任务 ID 并由后台轮询器更新状态，完成后通过内容接口读取视频。

## 开源协议

本集成不复制 New API 代码到画布仓库。画布源代码及其二次修改继续遵循上游 GNU AGPL v3 和原作者署名要求；对外提供修改版时应同步提供对应源代码和许可证信息。
