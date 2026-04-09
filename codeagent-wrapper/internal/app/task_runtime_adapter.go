package wrapper

import (
	"context"
	"fmt"
	"io"
	"strings"

	runtask "codeagent-wrapper/internal/application/runtask"
	runtaskset "codeagent-wrapper/internal/application/runtaskset"
	config "codeagent-wrapper/internal/config"
	executor "codeagent-wrapper/internal/executor"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type taskRunner func(TaskSpec, int) TaskResult

type singleTaskPlan = runtask.Plan
type parallelRunPlan = runtaskset.Plan

func defaultExecuteTaskLayers(parentCtx context.Context, layers [][]TaskSpec, maxWorkers int, runTask taskRunner) []TaskResult {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	if len(layers) == 1 && len(layers[0]) == 1 && strings.TrimSpace(layers[0][0].ID) == "" {
		task := layers[0][0]
		resultsCh := make(chan TaskResult, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					resultsCh <- runtaskset.PanicTaskResult(task.ID, r)
				}
			}()
			resultsCh <- runTask(task, noExecutionTimeout)
		}()
		select {
		case res := <-resultsCh:
			return []TaskResult{res}
		case <-parentCtx.Done():
			return []TaskResult{{TaskID: task.ID, ExitCode: 130, Error: "execution cancelled"}}
		}
	}
	return executor.ExecuteConcurrentWithContext(parentCtx, layers, noExecutionTimeout, maxWorkers, runTask)
}

var executeTaskLayersFn = defaultExecuteTaskLayers

func buildParallelRunPlan(cmd *cobra.Command, opts *cliOptions, v *viper.Viper) (parallelRunPlan, error) {
	backendName := defaultBackendName
	if cmd.Flags().Changed("backend") {
		backendName = strings.TrimSpace(opts.Backend)
		if backendName == "" {
			return parallelRunPlan{}, fmt.Errorf("--backend flag requires a value")
		}
	} else if val := strings.TrimSpace(v.GetString("backend")); val != "" {
		backendName = val
	}

	model := ""
	if cmd.Flags().Changed("model") {
		model = strings.TrimSpace(opts.Model)
		if model == "" {
			return parallelRunPlan{}, fmt.Errorf("--model flag requires a value")
		}
	} else {
		model = strings.TrimSpace(v.GetString("model"))
	}

	summaryOnly := !opts.FullOutput
	if !cmd.Flags().Changed("full-output") && v.IsSet("full-output") {
		summaryOnly = !v.GetBool("full-output")
	}

	outputPath := ""
	if cmd.Flags().Changed("output") {
		outputPath = strings.TrimSpace(opts.Output)
		if outputPath == "" {
			return parallelRunPlan{}, fmt.Errorf("--output flag requires a value")
		}
	} else if val := strings.TrimSpace(v.GetString("output")); val != "" {
		outputPath = val
	}

	skipChanged := cmd.Flags().Changed("skip-permissions") || cmd.Flags().Changed("dangerously-skip-permissions")
	skipPermissions := false
	if skipChanged {
		skipPermissions = opts.SkipPermissions
	} else {
		skipPermissions = v.GetBool("skip-permissions")
	}

	data, err := io.ReadAll(stdinReader)
	if err != nil {
		return parallelRunPlan{}, fmt.Errorf("failed to read stdin: %w", err)
	}

	return runtaskset.BuildPlan(runtaskset.BuildInput{
		BackendName:     backendName,
		Model:           model,
		OutputPath:      outputPath,
		SummaryOnly:     summaryOnly,
		SkipPermissions: skipPermissions,
		StdinData:       data,
		MaxWorkers:      resolveConfiguredMaxParallelWorkers(v),
	}, runtaskset.BuildDeps{
		ResolveBackendName: func(name string) (string, error) {
			backend, err := selectBackendFn(name)
			if err != nil {
				return "", err
			}
			return backend.Name(), nil
		},
		ParseConfig:     parseParallelConfig,
		TopologicalSort: topologicalSort,
	})
}

func runParallelPlan(parentCtx context.Context, plan parallelRunPlan) (string, int, error) {
	return runtaskset.RunPlan(parentCtx, plan, runtaskset.RunDeps{
		ExecuteConcurrent:         executeConcurrentWithContext,
		EnrichResults:             enrichTaskResults,
		RenderFinalOutputWithMode: generateFinalOutputWithMode,
		NoExecutionTimeout:        noExecutionTimeout,
	})
}

func prepareSingleTaskPlan(cfg *Config) (singleTaskPlan, error) {
	return runtask.PreparePlan(cfg, runtask.PrepareDeps{
		ResolveTaskText:      resolveSingleTaskText,
		ApplyPromptAndSkills: applySingleTaskPromptAndSkills,
		ShouldUseStdin:       shouldUseStdin,
		BuildCommandArgs:     buildCodexArgsFn,
	})
}

func executeSingleTaskPlan(parentCtx context.Context, plan singleTaskPlan) (TaskResult, error) {
	return runtask.ExecutePlan(parentCtx, plan, runtask.ExecuteDeps{
		ExecuteTaskLayers: func(parentCtx context.Context, layers [][]executor.TaskSpec, maxWorkers int, runTask func(executor.TaskSpec, int) executor.TaskResult) []executor.TaskResult {
			return executeTaskLayersFn(parentCtx, layers, maxWorkers, func(task TaskSpec, timeout int) TaskResult {
				return runTask(task, timeout)
			})
		},
		RunTask: func(task executor.TaskSpec, timeout int) executor.TaskResult {
			return runTaskFn(task, false, timeout)
		},
		EnrichResults: enrichTaskResults,
	})
}

func resolveSingleTaskText(cfg *config.Config) (taskText string, piped bool, err error) {
	if cfg.ExplicitStdin {
		logInfo("Explicit stdin mode: reading task from stdin")
		data, err := io.ReadAll(stdinReader)
		if err != nil {
			return "", false, fmt.Errorf("Failed to read stdin: %w", err)
		}
		taskText = string(data)
		if taskText == "" {
			return "", false, fmt.Errorf("Explicit stdin mode requires task input from stdin")
		}
		return taskText, !isTerminal(), nil
	}

	pipedTask, err := readPipedTask()
	if err != nil {
		return "", false, fmt.Errorf("Failed to read piped stdin: %w", err)
	}
	if pipedTask != "" {
		return pipedTask, true, nil
	}
	return cfg.Task, false, nil
}

func applySingleTaskPromptAndSkills(cfg *config.Config, taskText string) (string, error) {
	if strings.TrimSpace(cfg.PromptFile) != "" {
		prompt, err := readAgentPromptFile(cfg.PromptFile, cfg.PromptFileExplicit)
		if err != nil {
			return "", fmt.Errorf("Failed to read prompt file: %w", err)
		}
		taskText = wrapTaskWithAgentPrompt(prompt, taskText)
	}

	skills := cfg.Skills
	if len(skills) == 0 {
		skills = detectProjectSkills(cfg.WorkDir)
	}
	if len(skills) > 0 {
		if content := resolveSkillContent(skills, 0); content != "" {
			taskText += "\n\n# Domain Best Practices\n\n" + content
		}
	}
	return taskText, nil
}

func logSingleTaskStdinReasons(plan singleTaskPlan) {
	reasons := runtask.StdinReasons(plan)
	if len(reasons) == 0 {
		return
	}
	logWarn(fmt.Sprintf("Using stdin mode for task due to: %s", strings.Join(reasons, ", ")))
}

func enrichTaskResults(results []TaskResult) {
	for i := range results {
		results[i].CoverageTarget = defaultCoverageTarget
		if results[i].Message == "" {
			continue
		}

		lines := strings.Split(results[i].Message, "\n")
		results[i].Coverage = extractCoverageFromLines(lines)
		results[i].CoverageNum = extractCoverageNum(results[i].Coverage)
		results[i].FilesChanged = extractFilesChangedFromLines(lines)
		results[i].TestsPassed, results[i].TestsFailed = extractTestResultsFromLines(lines)
		results[i].KeyOutput = extractKeyOutputFromLines(lines, 0)
	}
}

func resolveConfiguredMaxParallelWorkers(v *viper.Viper) int {
	if v == nil || !v.IsSet("max-parallel-workers") {
		return config.ResolveMaxParallelWorkers()
	}
	return config.NormalizeMaxParallelWorkers(v.GetInt("max-parallel-workers"))
}
