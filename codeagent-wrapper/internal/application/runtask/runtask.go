package runtask

import (
	"context"
	"fmt"
	"strings"

	config "codeagent-wrapper/internal/config"
	executor "codeagent-wrapper/internal/executor"
)

type Plan struct {
	TaskSpec  executor.TaskSpec
	TaskText  string
	TargetArg string
	Piped     bool
	UseStdin  bool
	Explicit  bool
	Command   []string
}

type PrepareDeps struct {
	ResolveTaskText      func(*config.Config) (string, bool, error)
	ApplyPromptAndSkills func(*config.Config, string) (string, error)
	ShouldUseStdin       func(string, bool) bool
	BuildCommandArgs     func(*config.Config, string) []string
}

type ExecuteDeps struct {
	ExecuteTaskLayers func(context.Context, [][]executor.TaskSpec, int, func(executor.TaskSpec, int) executor.TaskResult) []executor.TaskResult
	RunTask           func(executor.TaskSpec, int) executor.TaskResult
	EnrichResults     func([]executor.TaskResult)
}

func PreparePlan(cfg *config.Config, deps PrepareDeps) (Plan, error) {
	taskText, piped, err := deps.ResolveTaskText(cfg)
	if err != nil {
		return Plan{}, err
	}

	taskText, err = deps.ApplyPromptAndSkills(cfg, taskText)
	if err != nil {
		return Plan{}, err
	}

	useStdin := cfg.ExplicitStdin || deps.ShouldUseStdin(taskText, piped)
	targetArg := taskText
	if useStdin {
		targetArg = "-"
	}

	taskSpec := executor.TaskSpec{
		Task:            taskText,
		WorkDir:         cfg.WorkDir,
		Mode:            cfg.Mode,
		SessionID:       cfg.SessionID,
		Backend:         cfg.Backend,
		Model:           cfg.Model,
		ReasoningEffort: cfg.ReasoningEffort,
		Agent:           cfg.Agent,
		SkipPermissions: cfg.SkipPermissions,
		Worktree:        cfg.Worktree,
		AllowedTools:    cfg.AllowedTools,
		DisallowedTools: cfg.DisallowedTools,
		UseStdin:        useStdin,
	}

	return Plan{
		TaskSpec:  taskSpec,
		TaskText:  taskText,
		TargetArg: targetArg,
		Piped:     piped,
		UseStdin:  useStdin,
		Explicit:  cfg.ExplicitStdin,
		Command:   deps.BuildCommandArgs(cfg, targetArg),
	}, nil
}

func ExecutePlan(parentCtx context.Context, plan Plan, deps ExecuteDeps) (executor.TaskResult, error) {
	results := deps.ExecuteTaskLayers(parentCtx, [][]executor.TaskSpec{{plan.TaskSpec}}, 1, func(task executor.TaskSpec, timeout int) executor.TaskResult {
		return deps.RunTask(task, timeout)
	})
	if len(results) != 1 {
		return executor.TaskResult{}, fmt.Errorf("unexpected single-task result count: %d", len(results))
	}
	deps.EnrichResults(results)
	return results[0], nil
}

func StdinReasons(plan Plan) []string {
	if !plan.UseStdin {
		return nil
	}

	var reasons []string
	if plan.Piped {
		reasons = append(reasons, "piped input")
	}
	if plan.Explicit {
		reasons = append(reasons, "explicit \"-\"")
	}
	if strings.Contains(plan.TaskText, "\n") {
		reasons = append(reasons, "newline")
	}
	if strings.Contains(plan.TaskText, "\\") {
		reasons = append(reasons, "backslash")
	}
	if strings.Contains(plan.TaskText, "\"") {
		reasons = append(reasons, "double-quote")
	}
	if strings.Contains(plan.TaskText, "'") {
		reasons = append(reasons, "single-quote")
	}
	if strings.Contains(plan.TaskText, "`") {
		reasons = append(reasons, "backtick")
	}
	if strings.Contains(plan.TaskText, "$") {
		reasons = append(reasons, "dollar")
	}
	if len(plan.TaskText) > 800 {
		reasons = append(reasons, "length>800")
	}
	return reasons
}
