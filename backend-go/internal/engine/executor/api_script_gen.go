package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// apiSpecScenario JSON spec 中单个场景的解析结构。
type apiSpecScenario struct {
	Name  string        `json:"name"`
	Steps []apiSpecStep `json:"steps"`
}

// apiSpecStep JSON spec 中单个步骤的解析结构。
type apiSpecStep struct {
	Action   string `json:"action"`
	Command  string `json:"command"`
	Expected string `json:"expected"`
}

// GenerateAPITestScript 从 API 集成测试 JSON spec 生成可执行 bash 脚本。
// 脚本特性：
//   - 每个场景封装为函数 scenario_N()，支持全量执行和单场景执行（SCENARIO_INDEX 环境变量）
//   - $BASE_URL 优先由调用方以环境变量注入；未注入时回退到部署时写入的固定存储
//     tests/.base_url（$(dirname $0)/../.base_url），脚本可脱离调用方独立运行
//   - stdout 输出结构化 JSON（ModuleTestResult 格式）
//   - exit code = 失败数（0 = 全通过）
func GenerateAPITestScript(specJSON string) (string, error) {
	specJSON = strings.TrimSpace(specJSON)
	if specJSON == "" {
		return "", fmt.Errorf("spec JSON 为空")
	}

	var spec struct {
		TestScenarios []apiSpecScenario `json:"testScenarios"`
	}
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return "", fmt.Errorf("解析 spec JSON 失败: %w", err)
	}
	if len(spec.TestScenarios) == 0 {
		return "", fmt.Errorf("testScenarios 为空")
	}

	var b strings.Builder

	// 脚本头部
	b.WriteString(`#!/bin/bash
set -o pipefail

# ============================================================
# API 集成测试脚本（由 loafer 后端自动生成，请勿手动编辑）
# 用法：
#   全量执行：BASE_URL=http://host:port bash this_script.sh
#   单场景：  BASE_URL=http://host:port SCENARIO_INDEX=0 bash this_script.sh
# BASE_URL 未注入时回退读取部署时写入的固定存储 tests/.base_url，
# 因此也可直接 bash this_script.sh 运行（需项目已部署过）。
# ============================================================

# 解析目标服务地址：优先调用方注入的 BASE_URL，未注入时回退到部署固定存储
if [ -z "${BASE_URL:-}" ]; then
  BASE_URL="$(cat "$(dirname "$0")/../.base_url" 2>/dev/null | tr -d '[:space:]')"
fi
if [ -z "$BASE_URL" ]; then
  echo "错误：未设置 BASE_URL，且未找到部署时写入的访问地址（$(dirname "$0")/../.base_url）。请先部署项目，或显式设置 BASE_URL=http://host:port 运行。" >&2
  exit 1
fi
SCENARIO_INDEX="${SCENARIO_INDEX:--1}"  # -1 = 全部执行

PASS_COUNT=0
FAIL_COUNT=0
TOTAL_COUNT=0
FAILURE_ITEMS=""
SCENARIO_ITEMS=""
STEPS_ITEMS=""

# JSON 转义：对整个字符串（含换行）做参数展开——先逃逸反斜杠、双引号，
# 再把换行替换为字面 \n。保证结果 JSON 单行、可被后端一次性解析。
# 不用 sed :a;N;$!ba 是因为单行无换行输入会在替换前提前退出，且无法处理内嵌换行。
json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/\\n}"
  printf '%s' "$s"
}

# 从 xtrace 输出中提取实际执行的顶层命令序列（去掉 eval 包装行）。
# xtrace 前缀 + 数量代表嵌套深度：eval 包装行为 D，命令顶层为 D+1，
# 命令内的 $(...) 等更深。用相对深度解析，脚本任意嵌套深度下都正确。
parse_actual_cmd() {
  local raw="$1"
  local eval_depth="" line d depth rest expanded=""
  while IFS= read -r line; do
    case "$line" in
      +*)
        d=${line%%[^+]*}
        depth=${#d}
        rest=${line:depth}
        rest=${rest# }
        if [ -z "$eval_depth" ]; then
          eval_depth=$depth
          continue
        fi
        if [ "$depth" -eq $((eval_depth + 1)) ]; then
          if [ -n "$expanded" ]; then expanded="${expanded}; "; fi
          expanded="${expanded}${rest}"
        fi
        ;;
    esac
  done <<< "$raw"
  printf '%s' "$expanded"
}

# 追加 failure 条目（用全局变量拼接 JSON 数组片段）
add_failure() {
  local kind="$1" name="$2" log="$3"
  local escaped_log
  escaped_log=$(printf '%s' "$log" | sed 's/\\/\\\\/g; s/"/\\"/g; s/\n/\\n/g' | head -c 2000)
  if [ -n "$FAILURE_ITEMS" ]; then
    FAILURE_ITEMS="${FAILURE_ITEMS},"
  fi
  FAILURE_ITEMS="${FAILURE_ITEMS}{\"kind\":\"${kind}\",\"name\":\"${name}\",\"log\":\"${escaped_log}\"}"
}

# 追加 scenario 条目（含每步骤明细 steps 数组，供前端展示可复现的输入/输出）
add_scenario() {
  local kind="$1" name="$2" passed="$3" log="$4"
  local escaped_log
  escaped_log=$(printf '%s' "$log" | sed 's/\\/\\\\/g; s/"/\\"/g; s/\n/\\n/g' | head -c 500)
  if [ -n "$SCENARIO_ITEMS" ]; then
    SCENARIO_ITEMS="${SCENARIO_ITEMS},"
  fi
  SCENARIO_ITEMS="${SCENARIO_ITEMS}{\"kind\":\"${kind}\",\"name\":\"${name}\",\"passed\":${passed},\"log\":\"${escaped_log}\",\"screenshot\":\"\",\"errorScreenshot\":\"\",\"steps\":[${STEPS_ITEMS}]}"
}

# 运行单个步骤：执行 curl 命令并检查 expected 子串，
# 同时记录「实际执行」的命令与输出——用 set -x 捕获展开后的真实值
#（$BASE_URL / $U / $(date ...) 等占位符替换为运行时实际值），而非原始模板，
# 保证步骤明细里展示的 command 可直接复现本次请求。
# 步骤明细 {action,command,ok,output,error} 追加进 STEPS_ITEMS（JSON 单行）。
# 返回 0=通过, 1=失败
run_step() {
  local idx="$1" action="$2" cmd="$3" expected="$4"
  local raw rc ok error expanded output item escaped_action escaped_cmd escaped_output escaped_error
  # 在子 shell 内开启 xtrace 执行命令：stdout+stderr+xtrace 一并捕获到 raw，
  # 事后按行分离——xtrace 行以 + 开头，其余为真实输出
  raw=$( { set -x; eval "$cmd"; } 2>&1 )
  rc=$?
  output=$(printf '%s\n' "$raw" | grep -v '^[+]')
  ok=true
  error=""
  if [ $rc -ne 0 ]; then
    ok=false
    error="命令退出码 $rc"
    echo "  ✗ 命令退出码 $rc: $(echo "$output" | tail -5)" >&2
  elif [ -n "$expected" ]; then
    if echo "$output" | grep -qF "$expected"; then
      echo "  ✓ 匹配: $expected" >&2
    else
      ok=false
      error="未匹配 expected「$expected」"
      echo "  ✗ 未匹配 expected「$expected」，实际输出: $(echo "$output" | head -c 500)" >&2
    fi
  fi
  expanded=$(parse_actual_cmd "$raw")
  # 先截断再转义，避免截断落在转义序列中间导致 JSON 非法
  escaped_action=$(json_escape "${action:0:200}")
  escaped_cmd=$(json_escape "${expanded:0:2000}")
  escaped_output=$(json_escape "${output:0:500}")
  escaped_error=$(json_escape "${error:0:300}")
  item=$(printf '{"action":"%s","command":"%s","ok":%s,"output":"%s","error":"%s"}' \
    "$escaped_action" "$escaped_cmd" "$ok" "$escaped_output" "$escaped_error")
  if [ -n "$STEPS_ITEMS" ]; then
    STEPS_ITEMS="${STEPS_ITEMS},"
  fi
  STEPS_ITEMS="${STEPS_ITEMS}${item}"
  [ "$ok" = true ]
}

`)

	// 每个场景生成一个函数
	for i, sc := range spec.TestScenarios {
		funcName := fmt.Sprintf("scenario_%d", i)
		b.WriteString(fmt.Sprintf("# 场景 %d: %s\n", i, sc.Name))
		b.WriteString(fmt.Sprintf("%s() {\n", funcName))
		b.WriteString(fmt.Sprintf("  local sc_name=%q\n", sc.Name))
		b.WriteString("  local sc_pass=true\n")
		b.WriteString("  local sc_log=\"\"\n")
		b.WriteString("  STEPS_ITEMS=\"\"\n") // 场景级步骤明细，本次执行前清空
		b.WriteString("  TOTAL_COUNT=$((TOTAL_COUNT+1))\n")
		b.WriteString("  echo \">>> 场景: ${sc_name}\" >&2\n\n")

		for j, step := range sc.Steps {
			b.WriteString(fmt.Sprintf("  # 步骤 %d: %s\n", j+1, step.Action))
			// 用 bash 单引号包装 action/cmd/expected：单引号内 $var、$(...) 不会在脚本解析期展开，
			// 而是保留到 run_step 内 eval 时才展开——保证命令中自带的变量赋值
			//（如 U="$(date +%s%N)" 后 -d "$U"）与 $BASE_URL 都能在运行时正确解析，
			// 而不是在构造 run_step 参数时被外层双引号提前展开成空串。
			// 内部单引号以 '\'' 转义（如 -H 'Content-Type: application/json'）。
			b.WriteString(fmt.Sprintf("  if ! run_step %d %s %s %s; then\n", j+1, bashSingleQuote(step.Action), bashSingleQuote(step.Command), bashSingleQuote(step.Expected)))
			b.WriteString(fmt.Sprintf("    sc_pass=false\n"))
			b.WriteString(fmt.Sprintf("    sc_log=%s\n", bashSingleQuote(fmt.Sprintf("步骤%d失败: %s", j+1, step.Action))))
			b.WriteString("  fi\n\n")
		}

		// 场景结束，记录结果
		b.WriteString(`  if [ "$sc_pass" = true ]; then
    PASS_COUNT=$((PASS_COUNT+1))
    add_scenario "api" "$sc_name" "true" "通过"
    echo "  ✓ 场景「${sc_name}」通过" >&2
  else
    FAIL_COUNT=$((FAIL_COUNT+1))
    add_failure "api" "$sc_name" "$sc_log"
    add_scenario "api" "$sc_name" "false" "$sc_log"
    echo "  ✗ 场景「${sc_name}」失败: ${sc_log}" >&2
  fi
}

`)

	}

	// 主执行逻辑
	b.WriteString(`# ============================================================
# 主逻辑：按 SCENARIO_INDEX 执行
# ============================================================

if [ "$SCENARIO_INDEX" -ge 0 ] 2>/dev/null; then
  # 单场景模式
  case "$SCENARIO_INDEX" in
`)
	for i := range spec.TestScenarios {
		b.WriteString(fmt.Sprintf("    %d) scenario_%d ;;\n", i, i))
	}
	b.WriteString(`    *) echo "SCENARIO_INDEX 越界: $SCENARIO_INDEX" >&2; exit 1 ;;
  esac
else
  # 全量执行
`)
	for i := range spec.TestScenarios {
		b.WriteString(fmt.Sprintf("  scenario_%d\n", i))
	}
	b.WriteString(`fi

# 输出结构化结果 JSON 到 stdout
PASSED=false
[ "$FAIL_COUNT" -eq 0 ] && PASSED=true
SUMMARY="API ${PASS_COUNT}/${TOTAL_COUNT} 通过"

printf '{"module_id":0,"passed":%s,"summary":"%s","failures":[%s],"scenarios":[%s]}\n' \
  "$PASSED" "$SUMMARY" "$FAILURE_ITEMS" "$SCENARIO_ITEMS"

exit "$FAIL_COUNT"
`)

	return b.String(), nil
}

// bashSingleQuote 把字符串包装为 bash 单引号字面量，内部单引号转义为 '\''。
// 与 %q（双引号）的关键差异：单引号内 $var、$(...) 不在脚本解析期展开，
// 而是保留到 run_step 内 eval 时才展开——保证命令中自带的变量赋值与 $BASE_URL
// 都能在运行时正确解析，而不是在构造 run_step 参数时被外层双引号提前展开成空串。
func bashSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// apiTestScriptPath 返回模块 API 测试脚本的绝对路径。
func apiTestScriptPath(workDir string, moduleID int64) string {
	return fmt.Sprintf("%s/tests/scripts/module-%d-api.sh", workDir, moduleID)
}

// writeAPITestScript 把生成的脚本写入 tests/scripts/ 目录（先建目录，写后可执行权限）。
func writeAPITestScript(workDir string, moduleID int64, script string) error {
	dir := fmt.Sprintf("%s/tests/scripts", workDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := apiTestScriptPath(workDir, moduleID)
	return os.WriteFile(path, []byte(script), 0o755)
}
