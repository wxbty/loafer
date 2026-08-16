package skill

import "testing"

// TestGlobalContainsEightSkills 内置注册表应包含 8 个职能 Skill。
func TestGlobalContainsEightSkills(t *testing.T) {
	if got := len(Global().List()); got != 8 {
		t.Fatalf("Global 应含 8 个 Skill，实际 %d", got)
	}
}

// TestBuiltinSkillNames 内置 Skill 应包含 8 个期望名称。
func TestBuiltinSkillNames(t *testing.T) {
	want := []string{
		"PlanSkill", "DecomposeSkill", "CodeSkill", "TestDesignSkill",
		"TestExecuteSkill", "FixSkill", "DeploySkill", "InfraVerifySkill",
	}
	names := Global().Names()
	for _, w := range want {
		if _, ok := Global().Get(w); !ok {
			t.Errorf("缺少内置 Skill %q，当前名称: %v", w, names)
		}
	}
}

// TestRegistryRegisterGetList 注册后可通过 Get/List 取回。
func TestRegistryRegisterGetList(t *testing.T) {
	r := NewRegistry()
	s := &Skill{Name: "FooSkill", Description: "测试"}
	if err := r.Register(s); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	got, ok := r.Get("FooSkill")
	if !ok || got != s {
		t.Fatalf("Get 未返回注册的 Skill")
	}
	if len(r.List()) != 1 {
		t.Fatalf("List 长度应为 1，实际 %d", len(r.List()))
	}
}

// TestRegistryDuplicateRejected 重复注册应报错，避免契约冲突。
func TestRegistryDuplicateRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Skill{Name: "Dup"}); err != nil {
		t.Fatalf("首次注册不应失败: %v", err)
	}
	if err := r.Register(&Skill{Name: "Dup"}); err == nil {
		t.Fatalf("重复注册应返回错误")
	}
}

// TestRegistryEmptyNameRejected 空名称注册应报错。
func TestRegistryEmptyNameRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&Skill{Name: ""}); err == nil {
		t.Fatalf("空名称注册应返回错误")
	}
	if err := r.Register(nil); err == nil {
		t.Fatalf("nil Skill 注册应返回错误")
	}
}

// TestRegistryGetMissing 缺失 Skill 应返回 ok=false。
func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("Nope"); ok {
		t.Fatalf("缺失 Skill 应返回 ok=false")
	}
}

// TestNamesSorted Names 应排序返回。
func TestNamesSorted(t *testing.T) {
	names := Global().Names()
	if len(names) == 0 {
		t.Fatalf("Names 不应为空")
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Fatalf("Names 应排序: %v", names)
		}
	}
}

// TestBuiltinSkillsContractComplete 对齐大赛要求：每个内置 Skill 的契约字段必须完整。
func TestBuiltinSkillsContractComplete(t *testing.T) {
	for _, s := range Global().List() {
		if s.Name == "" || s.Description == "" {
			t.Errorf("Skill 缺少名称/用途")
			continue
		}
		if len(s.Inputs) == 0 || len(s.Outputs) == 0 {
			t.Errorf("%s 缺少输入/输出", s.Name)
		}
		if s.CallConditions == "" {
			t.Errorf("%s 缺少调用条件", s.Name)
		}
		if len(s.Dependencies) == 0 {
			t.Errorf("%s 缺少依赖工具", s.Name)
		}
		if s.FailureHandling == "" {
			t.Errorf("%s 缺少失败处理", s.Name)
		}
		if s.VerificationMethod == "" {
			t.Errorf("%s 缺少验证方式", s.Name)
		}
		if s.SecurityBoundary == "" {
			t.Errorf("%s 缺少安全边界", s.Name)
		}
		if s.ReuseValue == "" {
			t.Errorf("%s 缺少复用价值", s.Name)
		}
		if s.Version == "" {
			t.Errorf("%s 缺少版本号", s.Name)
		}
		if s.VersionEvolution == "" {
			t.Errorf("%s 缺少版本演进策略", s.Name)
		}
		if s.OpenSourceDistribution == "" {
			t.Errorf("%s 缺少开源分发设计", s.Name)
		}
		if s.CollaborationFlow == "" {
			t.Errorf("%s 缺少与协同流程的关系", s.Name)
		}
		if s.Implementation == "" {
			t.Errorf("%s 缺少实现位置", s.Name)
		}
	}
}
