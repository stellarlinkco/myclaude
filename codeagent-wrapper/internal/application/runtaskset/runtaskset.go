package runtaskset

import (
	"context"
	"fmt"
	"strings"

	appoutput "codeagent-wrapper/internal/application/output"
	executor "codeagent-wrapper/internal/executor"
)

type Plan struct {
	OutputPath  string
	SummaryOnly bool
	Layers      [][]executor.TaskSpec
	MaxWorkers  int
}

type BuildInput struct {
	BackendName     string
	Model           string
	OutputPath      string
	SummaryOnly     bool
	SkipPermissions bool
	StdinData       []byte
	MaxWorkers      int
}

type BuildDeps struct {
	ResolveBackendName func(string) (string, error)
	ParseConfig        func([]byte) (*executor.ParallelConfig, error)
	TopologicalSort    func([]executor.TaskSpec) ([][]executor.TaskSpec, error)
}

type RunDeps struct {
	ExecuteConcurrent         func(context.Context, [][]executor.TaskSpec, int, int) []executor.TaskResult
	EnrichResults             func([]executor.TaskResult)
	RenderFinalOutputWithMode func([]executor.TaskResult, bool) string
	NoExecutionTimeout        int
}

func BuildPlan(input BuildInput, deps BuildDeps) (Plan, error) {
	backendName, err := deps.ResolveBackendName(input.BackendName)
	if err != nil {
		return Plan{}, err
	}

	cfg, err := deps.ParseConfig(input.StdinData)
	if err != nil {
		return Plan{}, err
	}

	cfg.GlobalBackend = backendName
	model := strings.TrimSpace(input.Model)
	for i := range cfg.Tasks {
		if strings.TrimSpace(cfg.Tasks[i].Backend) == "" {
			cfg.Tasks[i].Backend = backendName
		}
		if strings.TrimSpace(cfg.Tasks[i].Model) == "" && model != "" {
			cfg.Tasks[i].Model = model
		}
		cfg.Tasks[i].SkipPermissions = cfg.Tasks[i].SkipPermissions || input.SkipPermissions
	}

	layers, err := deps.TopologicalSort(cfg.Tasks)
	if err != nil {
		return Plan{}, err
	}

	return Plan{
		OutputPath:  input.OutputPath,
		SummaryOnly: input.SummaryOnly,
		Layers:      layers,
		MaxWorkers:  input.MaxWorkers,
	}, nil
}

func RunPlan(parentCtx context.Context, plan Plan, deps RunDeps) (string, int, error) {
	results := deps.ExecuteConcurrent(parentCtx, plan.Layers, deps.NoExecutionTimeout, plan.MaxWorkers)
	deps.EnrichResults(results)

	if err := appoutput.WriteStructuredOutput(plan.OutputPath, results); err != nil {
		return "", 0, err
	}

	exitCode := 0
	for _, res := range results {
		if res.ExitCode != 0 {
			exitCode = res.ExitCode
		}
	}

	return deps.RenderFinalOutputWithMode(results, plan.SummaryOnly), exitCode, nil
}

func PanicTaskResult(taskID string, recovered any) executor.TaskResult {
	return executor.TaskResult{TaskID: taskID, ExitCode: 1, Error: fmt.Sprintf("panic: %v", recovered)}
}
