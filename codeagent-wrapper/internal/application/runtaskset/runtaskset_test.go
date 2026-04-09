package runtaskset

import (
	"context"
	"testing"

	executor "codeagent-wrapper/internal/executor"
)

func TestBuildPlanAppliesGlobalDefaults(t *testing.T) {
	plan, err := BuildPlan(BuildInput{
		BackendName:     "codex",
		Model:           "gpt-5",
		OutputPath:      "out.json",
		SummaryOnly:     true,
		SkipPermissions: true,
		StdinData:       []byte("ignored"),
		MaxWorkers:      10,
	}, BuildDeps{
		ResolveBackendName: func(name string) (string, error) { return name, nil },
		ParseConfig: func([]byte) (*executor.ParallelConfig, error) {
			return &executor.ParallelConfig{Tasks: []executor.TaskSpec{{ID: "a", Task: "body"}}}, nil
		},
		TopologicalSort: func(tasks []executor.TaskSpec) ([][]executor.TaskSpec, error) {
			return [][]executor.TaskSpec{tasks}, nil
		},
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.MaxWorkers != 10 || len(plan.Layers) != 1 || plan.Layers[0][0].Backend != "codex" || plan.Layers[0][0].Model != "gpt-5" || !plan.Layers[0][0].SkipPermissions {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestRunPlanWritesAndRendersResults(t *testing.T) {
	out, exitCode, err := RunPlan(context.Background(), Plan{
		SummaryOnly: true,
		Layers:      [][]executor.TaskSpec{{{ID: "a"}}},
		MaxWorkers:  2,
	}, RunDeps{
		ExecuteConcurrent: func(context.Context, [][]executor.TaskSpec, int, int) []executor.TaskResult {
			return []executor.TaskResult{{TaskID: "a", ExitCode: 0, Message: "ok"}}
		},
		EnrichResults: func([]executor.TaskResult) {},
		RenderFinalOutputWithMode: func(results []executor.TaskResult, summaryOnly bool) string {
			if !summaryOnly || len(results) != 1 || results[0].TaskID != "a" {
				t.Fatalf("unexpected render args: %#v summaryOnly=%v", results, summaryOnly)
			}
			return "summary"
		},
	})
	if err != nil {
		t.Fatalf("RunPlan() error = %v", err)
	}
	if out != "summary" || exitCode != 0 {
		t.Fatalf("RunPlan() = (%q, %d), want (%q, 0)", out, exitCode, "summary")
	}
}
