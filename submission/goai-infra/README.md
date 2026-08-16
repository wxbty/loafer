# GOAI Agent Infra 初赛提交材料

本目录包含 GOAI 世界人工智能开源大赛 Agent Infra 赛道初赛所需的提交材料。

## 文件清单

| 文件 | 用途 | 说明 |
|------|------|------|
| `work-introduction.txt` | 作品简介（提交用） | 可直接复制到报名系统的 500 字以内作品简介 |
| `work-introduction.md` | 作品简介（带格式说明） | 包含字数统计与项目名称说明 |
| `proposal-ppt.pptx` | **方案 PPT（正式 PPT 文件）** | 可直接作为附件提交，用 PowerPoint/WPS 打开编辑 |
| `proposal-ppt.pdf` | 方案 PPT（PDF 版） | 供不需编辑的场合直接提交 |
| `proposal-ppt-outline.md` | 方案 PPT 逐页脚本 | 每页标题与要点，用于快速修改文案 |
| `proposal-ppt.html` | 方案 PPT（网页版） | 用浏览器打开即可全屏演示 |
| `framework-mapping.md` | 项目与比赛框架映射 | 详细说明 Agent、Skill、MCP、可观测、RAG 如何对应比赛要求 |

## 快速使用

1. **作品简介**：复制 `work-introduction.txt` 中的内容到报名系统。
2. **方案 PPT**：直接使用 `proposal-ppt.pptx`（PowerPoint/WPS 打开可编辑），或提交 `proposal-ppt.pdf`。
3. **补充材料**：如需在 PPT 中补充技术细节，参考 `framework-mapping.md`。

## 重要提醒

- 初赛截止时间为 **2026-08-16**，请尽快完成报名系统提交。
- 作品简介需控制在 **500 字以内**，项目名建议约 **20 字**。
- 方案 PPT 建议格式为 **PPT 或 PDF**。
- 如有代码仓库需提交，可额外提供仓库链接（初赛非强制）。

## 项目信息

- 项目名称：Loafer 多 Agent 软件交付平台
- 项目仓库：[待填写，如需要可补充]
- 技术栈：Go + Gin + GORM + MySQL / Vue 3 + TypeScript + Element Plus
- 当前状态：已实现从需求输入到自动部署的全链路，支持自动测试门禁、失败自动修复、断点续跑
- Skill 层：`backend-go/internal/engine/skill` 能力契约注册表 + `GET /projects/:id/available-skills` API（8 个职能 Skill）
