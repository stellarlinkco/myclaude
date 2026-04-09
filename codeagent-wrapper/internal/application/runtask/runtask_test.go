package runtask

import (
	"context"
	"strings"
	"testing"

	config "codeagent-wrapper/internal/config"
	executor "codeagent-wrapper/internal/executor"
)

func TestPreparePlanBuildsTaskSpecAndCommand(t *testing.T) {
	cfg := &config.Config{Task: "body", Backend: "codex", WorkDir: ".", ExplicitStdin: false}

	plan, err := PreparePlan(cfg, PrepareDeps{
		ResolveTaskText: func(*config.Config) (string, bool, error) { return "body", false, nil },
		ApplyPromptAndSkills: func(_ *config.Config, task string) (string, error) {
			return task + "\nextra", nil
		},
		ShouldUseStdin:   func(task string, piped bool) bool { return strings.Contains(task, "\n") || piped },
		BuildCommandArgs: func(_ *config.Config, target string) []string { return []string{"exec", target} },
	})
	if err != nil {
		t.Fatalf("PreparePlan() error = %v", err)
	}
	if !plan.UseStdin || plan.TargetArg != "-" {
		t.Fatalf("plan = %#v, want stdin target '-'", plan)
	}
	if plan.TaskSpec.Task != "body\nextra" {
		t.Fatalf("task = %q, want wrapped task", plan.TaskSpec.Task)
	}
	if len(plan.Command) != 2 || plan.Command[1] != "-" {
		t.Fatalf("command = %#v, want target '-'", plan.Command)
	}
}

func TestExecutePlanUsesSharedExecutor(t *testing.T) {
	plan := Plan{TaskSpec: executor.TaskSpec{Task: "body"}}
	var gotLayers [][]executor.TaskSpec
	var gotTask executor.TaskSpec

	result, err := ExecutePlan(context.Background(), plan, ExecuteDeps{
		ExecuteTaskLayers: func(_ context.Context, layers [][]executor.TaskSpec, maxWorkers int, runTask func(executor.TaskSpec, int) executor.TaskResult) []executor.TaskResult {
			gotLayers = layers
			return []executor.TaskResult{runTask(layers[0][0], 0)}
		},
		RunTask: func(task executor.TaskSpec, timeout int) executor.TaskResult {
			gotTask = task
			return executor.TaskResult{Message: "ok"}
		},
		EnrichResults: func([]executor.TaskResult) {},
	})
	if err != nil {
		t.Fatalf("ExecutePlan() error = %v", err)
	}
	if result.Message != "ok" || gotTask.Task != "body" || len(gotLayers) != 1 || len(gotLayers[0]) != 1 {
		t.Fatalf("unexpected result=%#v task=%#v layers=%#v", result, gotTask, gotLayers)
	}
}
