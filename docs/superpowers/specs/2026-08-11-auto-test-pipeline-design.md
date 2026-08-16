# 流水线全自动测试闭环设计

日期：2026-08-11
状态：已批准（用户逐节确认）

## 背景与目标

当前 loafer 流水线在每个业务模块任务跑完后将模块置为「待测试」(status=2) 并暂停，等待人工在 UI 测试并标记完成。设计初衷是**全自动**：模块开发完成后由测试 agent 自动执行集成测试和 Playwright UI 测试，通过则继续下一模块，失败则自动修复重试，全程无需人工暂停。

现状关键事实：

- `handler/pipeline.go:1028-1037`：业务模块任务完成后置 status=2 并 `return` 暂停流水线。
- `handler/pipeline.go:965-974`：重启时遇到 status=2/3 的模块同样暂停（本次改造后此分支语义改变）。
- `handler/pipeline.go:960`：`mod.Status >= 4` 把测试失败(5)、失败(6) 误判为已完成而跳过，使 977 行重跑分支成为死代码——**本次一并修复**。
- TDD 相关接口（`RunAllAssertions`、`FixAllStream`、`GenerateScenarioStream` 等，`handler/module.go:291-436`）全部是桩，返回「暂未实现」，本设计不依赖它们。
- 可复用件：`PlaywrightService.RunTest`（跑 `npx playwright test` 并解析结果）、`GenerateTestSpec`（生成 playwright.config.ts）、`TaskExecutor`/`OfflineExecutor`（Claude CLI 任务执行通道）、`deployService.Deploy`（全量构建+重启+Nginx）、`InfraVerify`（基础架构模块构建+启动校验）。
- 流水线尾部「阶段5 测试验证」目前是假检查（部署记录存在即通过）。

## 已确认的决策

1. **测试时机**：每个业务模块完成后立即部署+测试，通过才继续下一模块。
2. **测试来源**：独立测试 agent（Claude CLI）现场编写并运行测试，流水线不依赖分解阶段生成的测试任务。
3. **失败处理**：自动修复最多 3 轮，耗尽后模块 status=5，流水线暂停等人工。
4. **每模块部署方式**：复用现有 `deployService.Deploy` 全量重部署，不做增量优化。
5. **结果契约**：测试 agent 将结构化结果写入 JSON 文件，后端读文件判定，不解析 stdout。

## 整体流程

业务模块任务执行完成后进入全自动闭环：

```
业务模块任务执行完成
  → ① 全量部署（复用 deployService.Deploy：build + 重启 + Nginx）
  → ② 测试 agent（新 TestExecutor，Claude CLI）现场编写并运行
       集成测试（go test / API 级）+ Playwright UI 用例，
       结果写入 tests/results/module-<id>.json
  → ③ 后端读取 JSON 判定：
       通过 → 模块 status=4（完成）→ 继续下一模块
       失败 → ④ 修复 agent（复用 CLI 通道）读失败明细改代码
              → 回到 ①（最多 3 轮）
       3 轮耗尽 → 模块 status=5（测试失败）→ 流水线 paused，
                  输出完整失败报告，等人工介入后重启流水线续跑
```

- 基础架构模块维持现状：任务完成后自动 InfraVerify（构建+启动校验），失败即终止流水线。
- 流水线尾部「阶段4 部署上线」保留（全局最终部署）；「阶段5 测试验证」的假检查改为跑一次全局 Playwright 冒烟。

## 组件划分

| 组件 | 位置 | 职责 | 依赖 |
|---|---|---|---|
| `TestExecutor`（新增） | `backend-go/internal/engine/executor/test_executor.go` | 构造测试 prompt、调 `OfflineExecutor` 跑测试 agent、读取并解析 JSON 结果文件、每轮结果落库为 `TestRun`（`test_type="module-auto"`） | OfflineExecutor、db、cfg |
| 修复执行器 | 并入 `TestExecutor`（方法级拆分） | 构造修复 prompt（携带上一轮 failures 全文），调 CLI 改代码 | OfflineExecutor |
| `PipelineHandler` 改造 | `backend-go/internal/handler/pipeline.go` | 模块循环内用「部署→测试→修复」闭环替换 status=2 暂停分支；修复 status>=4 误判 | TestExecutor、deployService |
| 结果契约 | 项目工作目录 `tests/results/module-<id>.json` | 测试 agent 与后端之间的结构化接口 | — |
| 模块状态机 | 复用现有 0-6 | 测试中=3、完成=4、测试失败=5 由闭环自动驱动 | — |

## 数据流与状态流转

模块状态机（自动驱动）：

```
status=0 待执行
  → 任务执行中 status=1
  → 任务完成，进入测试闭环 status=3（测试中）
  → 测试通过 status=4（完成）→ 下一模块
  → 测试失败且轮次未耗尽：保持 status=3，继续修复循环
  → 3 轮耗尽 status=5（测试失败）→ 流水线 paused
```

断点续跑语义（重启流水线时按模块最新状态分流）：

- `status == 4` → 跳过（修复 `>= 4` 误判后的条件）
- `status == 5 / 6` → 重跑该模块任务后重新进入测试闭环（原设计意图，修复死代码后生效）
- `status == 3`（上次在测试闭环中途被中断）→ 不重跑任务，直接从「部署→测试」环节恢复
- `status == 2`（历史遗留待测试模块）→ 同样直接进入「部署→测试」环节，兼容旧数据

结果 JSON 契约 `tests/results/module-<id>.json`：

```json
{
  "module_id": 12,
  "round": 2,
  "passed": false,
  "summary": "集成测试 8/10 通过；Playwright 3/4 通过",
  "failures": [
    {"kind": "integration", "name": "TestLoginAPI", "log": "..."},
    {"kind": "e2e", "name": "登录流程.spec.ts", "log": "..."}
  ]
}
```

每轮结果同时落库为 `TestRun` 记录，UI 测试历史中可见每轮详情。修复 agent 的 prompt 携带上一轮 `failures` 数组全文。

## 错误处理

| 故障 | 处理 |
|---|---|
| 部署失败（build/启动不过） | 视为一轮失败：错误输出喂给修复 agent，计入 3 轮额度 |
| 测试 agent CLI 崩溃/超时 | 同上当一轮失败处理；JSON 文件缺失或解析失败也按本轮失败处理，log 取 agent 原始输出尾部 |
| 修复 agent 未改出有效修复 | 不特殊处理——下一轮测试自然再次失败，轮次耗尽后暂停 |
| 3 轮耗尽 | status=5，流水线 paused，输出三轮完整失败报告；人工在 UI 修复/调整后重启流水线从该模块恢复 |
| CLI 整体不可用 | 维持现有脚手架降级路径，测试闭环跳过（脚手架模式本就不支持 CLI 任务执行） |

UI 手动「标记完成」按钮保留，作为人工兜底（可把 status=5 模块手动置 4 跳过）。

## 测试策略（对本改动自身）

- 单元测试（Go）：
  - `test_executor_test.go`：JSON 结果解析（缺失/畸形/通过/失败）、prompt 构造、轮次判定
  - pipeline 模块循环分支逻辑：status=4 跳过、5/6 重跑、2/3 进测试闭环（用 fake TestExecutor/deploy 注入）
- 集成验证：在 loafer 自身上跑一个小项目（2 个业务模块），观察闭环日志、状态流转、TestRun 落库
- 提交前 `go build ./...` 与 `npm run build` 均通过

## 明确不做（YAGNI）

- 增量部署/前端变更检测：不做，全量重部署。
- 实现 TDD 桩接口（RunAllAssertions 等）：不在本次范围，UI 对应入口维持现状。
- 测试 agent 的多模型/多轮对话能力：单轮 prompt，修复通过新 prompt 携带上下文实现。
