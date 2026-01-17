package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"okrchestra/internal/adapters"
	"okrchestra/internal/audit"
	"okrchestra/internal/daemon"
	"okrchestra/internal/metrics"
	"okrchestra/internal/okrstore"
	"okrchestra/internal/planner"
	"okrchestra/internal/workspace"
)

const appName = "okrchestra"

func main() {
	flag.String("workspace", "", "Path to workspace root")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s: OKR-driven agent orchestration\n\n", appName)
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [command] [flags]\n\n", appName)
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  agent   Manage agents")
		fmt.Fprintln(os.Stderr, "  daemon  Manage daemon")
		fmt.Fprintln(os.Stderr, "  init    Initialize a new workspace")
		fmt.Fprintln(os.Stderr, "  okr     Manage OKRs")
		fmt.Fprintln(os.Stderr, "  kr      Manage key results")
		fmt.Fprintln(os.Stderr, "  plan    Manage plans")
		fmt.Fprintln(os.Stderr, "  help    Show this help")
		fmt.Fprintln(os.Stderr, "\nFlags:")
		flag.PrintDefaults()
	}

	workspacePath, remaining, err := extractWorkspaceFlag(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	args := remaining
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		flag.Usage()
		return
	}

	switch args[0] {
	case "agent":
		if err := runAgent(args[1:], workspacePath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "daemon":
		if err := runDaemon(args[1:], workspacePath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "init":
		if err := runInit(args[1:], workspacePath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "okr":
		if err := runOKR(args[1:], workspacePath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "kr":
		if err := runKR(args[1:], workspacePath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "plan":
		if err := runPlan(args[1:], workspacePath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
		flag.Usage()
		os.Exit(1)
	}
}

type workspaceOverrides struct {
	OKRsDir      string
	CultureDir   string
	MetricsDir   string
	ArtifactsDir string
	AuditDB      string
}

type resolvedWorkspace struct {
	Workspace    *workspace.Workspace
	OKRsDir      string
	CultureDir   string
	MetricsDir   string
	ArtifactsDir string
	AuditDB      string
}

func resolveWorkspaceAndOverrides(root string, overrides workspaceOverrides) (*resolvedWorkspace, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("--workspace is required")
	}
	ws, err := workspace.Resolve(root)
	if err != nil {
		return nil, err
	}
	resolved := &resolvedWorkspace{Workspace: ws}
	resolved.OKRsDir = ws.OKRsDir
	resolved.CultureDir = ws.CultureDir
	resolved.MetricsDir = ws.MetricsDir
	resolved.ArtifactsDir = ws.ArtifactsDir
	resolved.AuditDB = ws.AuditDBPath

	if overrides.OKRsDir != "" {
		resolved.OKRsDir, err = ws.ResolvePath(overrides.OKRsDir)
		if err != nil {
			return nil, fmt.Errorf("resolve --okrs-dir: %w", err)
		}
	}
	if overrides.CultureDir != "" {
		resolved.CultureDir, err = ws.ResolvePath(overrides.CultureDir)
		if err != nil {
			return nil, fmt.Errorf("resolve --culture-dir: %w", err)
		}
	}
	if overrides.MetricsDir != "" {
		resolved.MetricsDir, err = ws.ResolvePath(overrides.MetricsDir)
		if err != nil {
			return nil, fmt.Errorf("resolve --metrics-dir: %w", err)
		}
	}
	if overrides.ArtifactsDir != "" {
		resolved.ArtifactsDir, err = ws.ResolvePath(overrides.ArtifactsDir)
		if err != nil {
			return nil, fmt.Errorf("resolve --artifacts-dir: %w", err)
		}
	}
	if overrides.AuditDB != "" {
		resolved.AuditDB, err = ws.ResolvePath(overrides.AuditDB)
		if err != nil {
			return nil, fmt.Errorf("resolve --audit-db: %w", err)
		}
	}
	return resolved, nil
}

func extractWorkspaceFlag(args []string) (string, []string, error) {
	var workspacePath string
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--workspace" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--workspace requires a value")
			}
			workspacePath = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--workspace=") {
			workspacePath = strings.TrimPrefix(arg, "--workspace=")
			continue
		}
		remaining = append(remaining, arg)
	}
	return workspacePath, remaining, nil
}

func runAgent(args []string, workspacePath string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return fmt.Errorf("%s agent: missing subcommand", appName)
	}

	switch args[0] {
	case "run":
		return runAgentRun(args[1:], workspacePath)
	default:
		return fmt.Errorf("%s agent: unknown subcommand %q", appName, args[0])
	}
}

func runAgentRun(args []string, workspacePath string) error {
	fs := flag.NewFlagSet("agent run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	adapterName := fs.String("adapter", "codex", "Adapter name")
	promptPath := fs.String("prompt", "", "Path to prompt file")
	workDir := fs.String("workdir", "", "Working directory (default: <workspace>)")
	artifactsDir := fs.String("artifacts", "", "Artifacts directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	resolved, err := resolveWorkspaceAndOverrides(workspacePath, workspaceOverrides{})
	if err != nil {
		return err
	}
	if err := resolved.Workspace.EnsureDirs(); err != nil {
		return err
	}

	if *promptPath == "" {
		return fmt.Errorf("prompt is required")
	}
	if *artifactsDir == "" {
		return fmt.Errorf("artifacts dir is required")
	}

	absPrompt, err := resolved.Workspace.ResolvePath(*promptPath)
	if err != nil {
		return fmt.Errorf("resolve prompt path: %w", err)
	}
	if *workDir == "" {
		*workDir = resolved.Workspace.Root
	}
	absWorkDir, err := resolved.Workspace.ResolvePath(*workDir)
	if err != nil {
		return fmt.Errorf("resolve workdir: %w", err)
	}
	absArtifactsDir, err := resolved.Workspace.ResolvePath(*artifactsDir)
	if err != nil {
		return fmt.Errorf("resolve artifacts dir: %w", err)
	}

	cfg := adapters.RunConfig{
		PromptPath:   absPrompt,
		WorkDir:      absWorkDir,
		ArtifactsDir: absArtifactsDir,
	}

	var adapter adapters.AgentAdapter
	switch *adapterName {
	case "codex":
		adapter = &adapters.CodexAdapter{}
	case "mock":
		adapter = &adapters.MockAdapter{}
	default:
		return fmt.Errorf("unknown adapter: %s", *adapterName)
	}

	logger := audit.NewLogger(resolved.AuditDB)
	startPayload := map[string]any{
		"workspace": resolved.Workspace.Root,
		"adapter":   adapter.Name(),
		"prompt":    absPrompt,
		"workdir":   absWorkDir,
		"artifacts": absArtifactsDir,
	}
	if err := logger.LogEvent("cli", "agent_run_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	ctx := context.Background()
	result, runErr := adapter.Run(ctx, cfg)

	finishPayload := map[string]any{
		"adapter":   adapter.Name(),
		"prompt":    absPrompt,
		"workdir":   absWorkDir,
		"artifacts": absArtifactsDir,
	}
	if result != nil {
		finishPayload["exit_code"] = result.ExitCode
		finishPayload["transcript"] = result.TranscriptPath
		finishPayload["summary"] = result.SummaryPath
	}
	if runErr != nil {
		finishPayload["error"] = runErr.Error()
	}
	if err := logger.LogEvent("cli", "agent_run_finished", finishPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	return runErr
}

func runOKR(args []string, workspacePath string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return fmt.Errorf("%s okr: missing subcommand", appName)
	}

	switch args[0] {
	case "propose":
		return runOKRPropose(args[1:], workspacePath)
	case "apply":
		return runOKRApply(args[1:], workspacePath)
	default:
		return fmt.Errorf("%s okr: unknown subcommand %q", appName, args[0])
	}
}

func runKR(args []string, workspacePath string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return fmt.Errorf("%s kr: missing subcommand", appName)
	}

	switch args[0] {
	case "measure":
		return runKRMeasure(args[1:], workspacePath)
	case "score":
		return runKRScore(args[1:], workspacePath)
	default:
		return fmt.Errorf("%s kr: unknown subcommand %q", appName, args[0])
	}
}

func runPlan(args []string, workspacePath string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return fmt.Errorf("%s plan: missing subcommand", appName)
	}

	switch args[0] {
	case "generate":
		return runPlanGenerate(args[1:], workspacePath)
	case "run":
		return runPlanRun(args[1:], workspacePath)
	default:
		return fmt.Errorf("%s plan: unknown subcommand %q", appName, args[0])
	}
}

func runInit(args []string, workspacePath string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	template := fs.String("template", "minimal", "Workspace template (default: minimal)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *template != "minimal" {
		return fmt.Errorf("unknown template: %s", *template)
	}
	if strings.TrimSpace(workspacePath) == "" {
		return fmt.Errorf("--workspace is required")
	}

	root, err := workspace.ResolveRoot(workspacePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create workspace root: %w", err)
	}
	ws, err := workspace.Resolve(root)
	if err != nil {
		return err
	}

	logger := audit.NewLogger(ws.AuditDBPath)
	startPayload := map[string]any{
		"workspace": ws.Root,
		"template":  *template,
	}
	if err := logger.LogEvent("cli", "workspace_init_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}
	var finishErr error
	defer func() {
		finishPayload := map[string]any{
			"workspace": ws.Root,
			"template":  *template,
		}
		if finishErr != nil {
			finishPayload["error"] = finishErr.Error()
		}
		_ = logger.LogEvent("cli", "workspace_init_finished", finishPayload)
	}()

	dirs := []string{
		ws.OKRsDir,
		ws.CultureDir,
		ws.MetricsDir,
		ws.ArtifactsDir,
		ws.AuditDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			finishErr = fmt.Errorf("create %s: %w", dir, err)
			return finishErr
		}
	}
	if err := ws.EnsureDirs(); err != nil {
		finishErr = err
		return finishErr
	}

	if err := writeFileIfMissing(filepath.Join(ws.CultureDir, "values.md"), minimalValuesTemplate); err != nil {
		finishErr = err
		return finishErr
	}
	if err := writeFileIfMissing(filepath.Join(ws.CultureDir, "standards.md"), minimalStandardsTemplate); err != nil {
		finishErr = err
		return finishErr
	}
	if err := writeFileIfMissing(filepath.Join(ws.OKRsDir, "org.yml"), minimalOrgTemplate); err != nil {
		finishErr = err
		return finishErr
	}
	if err := writeFileIfMissing(filepath.Join(ws.OKRsDir, "permissions.yml"), minimalPermissionsTemplate); err != nil {
		finishErr = err
		return finishErr
	}
	if err := writeFileIfMissing(filepath.Join(ws.MetricsDir, "manual.yml"), minimalManualMetricsTemplate); err != nil {
		finishErr = err
		return finishErr
	}
	if err := writeFileIfMissing(filepath.Join(ws.MetricsDir, "ci_report.json"), minimalCIReportTemplate); err != nil {
		finishErr = err
		return finishErr
	}

	fmt.Fprintf(os.Stdout, "Initialized workspace: %s\n", ws.Root)
	fmt.Fprintln(os.Stdout, "Next steps:")
	fmt.Fprintf(os.Stdout, "  %s kr measure --workspace %s\n", appName, ws.Root)
	fmt.Fprintf(os.Stdout, "  %s plan generate --workspace %s\n", appName, ws.Root)
	fmt.Fprintf(os.Stdout, "  %s plan run --workspace %s --adapter mock artifacts/plans/<date>/plan.json\n", appName, ws.Root)
	return nil
}

func runPlanGenerate(args []string, workspacePath string) error {
	fs := flag.NewFlagSet("plan generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	okrsDir := fs.String("okrs-dir", "", "Path to OKR YAML directory (default: <workspace>/okrs)")
	cultureDir := fs.String("culture-dir", "", "Path to culture directory (default: <workspace>/culture)")
	metricsDir := fs.String("metrics-dir", "", "Path to metrics directory (default: <workspace>/metrics)")
	artifactsDir := fs.String("artifacts-dir", "", "Path to artifacts directory (default: <workspace>/artifacts)")
	auditDB := fs.String("audit-db", "", "Path to audit SQLite DB (default: <workspace>/audit/audit.sqlite)")
	outDir := fs.String("out-dir", "", "Base directory to write plans (default: <workspace>/artifacts/plans)")
	asOfStr := fs.String("as-of", "", "As-of date (YYYY-MM-DD, default: today UTC)")
	objectiveID := fs.String("objective-id", "", "Optional objective_id to target")
	krID := fs.String("kr-id", "", "Optional kr_id to target")
	agentRole := fs.String("agent-role", "software_engineer", "Agent role for generated items")

	if err := fs.Parse(args); err != nil {
		return err
	}

	resolved, err := resolveWorkspaceAndOverrides(workspacePath, workspaceOverrides{
		OKRsDir:      *okrsDir,
		CultureDir:   *cultureDir,
		MetricsDir:   *metricsDir,
		ArtifactsDir: *artifactsDir,
		AuditDB:      *auditDB,
	})
	if err != nil {
		return err
	}
	if err := resolved.Workspace.EnsureDirs(); err != nil {
		return err
	}
	*okrsDir = resolved.OKRsDir
	if *outDir == "" {
		*outDir = filepath.Join(resolved.ArtifactsDir, "plans")
	} else {
		*outDir, err = resolved.Workspace.ResolvePath(*outDir)
		if err != nil {
			return fmt.Errorf("resolve --out-dir: %w", err)
		}
	}

	asOf := time.Now().UTC().Truncate(24 * time.Hour)
	if *asOfStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02", *asOfStr, time.UTC)
		if err != nil {
			return fmt.Errorf("parse --as-of: %w", err)
		}
		asOf = parsed.UTC().Truncate(24 * time.Hour)
	}

	logger := audit.NewLogger(resolved.AuditDB)
	startPayload := map[string]any{
		"workspace":    resolved.Workspace.Root,
		"okrs_dir":     *okrsDir,
		"out_dir":      *outDir,
		"as_of":        asOf.Format("2006-01-02"),
		"objective_id": *objectiveID,
		"kr_id":        *krID,
		"agent_role":   *agentRole,
		"command":      "plan generate",
	}
	if err := logger.LogEvent("cli", "plan_generate_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	res, err := planner.GeneratePlan(planner.GenerateOptions{
		OKRsDir:       *okrsDir,
		OutputBaseDir: *outDir,
		AsOf:          asOf,
		ObjectiveID:   *objectiveID,
		KRID:          *krID,
		AgentRole:     *agentRole,
	})

	finishPayload := map[string]any{
		"okrs_dir": *okrsDir,
		"out_dir":  *outDir,
	}
	if err != nil {
		finishPayload["error"] = err.Error()
		_ = logger.LogEvent("cli", "plan_generate_finished", finishPayload)
		return err
	}

	finishPayload["plan_path"] = res.PlanPath
	finishPayload["plan_id"] = res.Plan.ID
	_ = logger.LogEvent("cli", "plan_generate_finished", finishPayload)

	fmt.Fprintf(os.Stdout, "Wrote plan: %s\n", res.PlanPath)
	return nil
}

func runPlanRun(args []string, workspacePath string) error {
	if len(args) == 0 {
		return fmt.Errorf("plan path is required")
	}

	planArg := ""
	remaining := args
	if len(remaining) > 0 && !strings.HasPrefix(remaining[0], "-") {
		planArg = remaining[0]
		remaining = remaining[1:]
	}

	fs := flag.NewFlagSet("plan run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	adapterName := fs.String("adapter", "codex", "Adapter name")
	okrsDir := fs.String("okrs-dir", "", "Path to OKR YAML directory (default: <workspace>/okrs)")
	cultureDir := fs.String("culture-dir", "", "Path to culture directory (default: <workspace>/culture)")
	metricsDir := fs.String("metrics-dir", "", "Path to metrics directory (default: <workspace>/metrics)")
	artifactsDir := fs.String("artifacts-dir", "", "Path to artifacts directory (default: <workspace>/artifacts)")
	auditDB := fs.String("audit-db", "", "Path to audit SQLite DB (default: <workspace>/audit/audit.sqlite)")
	workDir := fs.String("workdir", "", "Working directory (default: <workspace>)")
	timeout := fs.Duration("timeout", 0, "Optional per-item timeout (e.g. 10m)")
	follow := fs.Bool("follow", false, "Stream agent transcript.log while running")
	followLines := fs.Int("follow-lines", 200, "When following, start from last N lines (0 = from start)")
	if err := fs.Parse(remaining); err != nil {
		return err
	}
	if planArg == "" {
		rest := fs.Args()
		if len(rest) == 0 {
			return fmt.Errorf("plan path is required")
		}
		planArg = rest[0]
	}

	resolved, err := resolveWorkspaceAndOverrides(workspacePath, workspaceOverrides{
		OKRsDir:      *okrsDir,
		CultureDir:   *cultureDir,
		MetricsDir:   *metricsDir,
		ArtifactsDir: *artifactsDir,
		AuditDB:      *auditDB,
	})
	if err != nil {
		return err
	}
	if *workDir == "" {
		*workDir = resolved.Workspace.Root
	}
	if err := resolved.Workspace.EnsureDirs(); err != nil {
		return err
	}

	if !filepath.IsAbs(planArg) {
		planArg, err = resolved.Workspace.ResolvePath(planArg)
		if err != nil {
			return fmt.Errorf("resolve plan path: %w", err)
		}
	}

	absPlan, err := filepath.Abs(planArg)
	if err != nil {
		return fmt.Errorf("resolve plan path: %w", err)
	}
	absWorkDir, err := resolved.Workspace.ResolvePath(*workDir)
	if err != nil {
		return fmt.Errorf("resolve workdir: %w", err)
	}

	var adapter adapters.AgentAdapter
	switch *adapterName {
	case "codex":
		adapter = &adapters.CodexAdapter{}
	case "mock":
		adapter = &adapters.MockAdapter{}
	default:
		return fmt.Errorf("unknown adapter: %s", *adapterName)
	}

	logger := audit.NewLogger(resolved.AuditDB)
	startPayload := map[string]any{
		"workspace": resolved.Workspace.Root,
		"plan":      absPlan,
		"adapter":   adapter.Name(),
		"workdir":   absWorkDir,
		"timeout":   timeout.String(),
	}
	if err := logger.LogEvent("cli", "plan_run_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	ctx := context.Background()
	res, runErr := planner.RunPlan(ctx, planner.RunOptions{
		PlanPath:          absPlan,
		WorkDir:           absWorkDir,
		Adapter:           adapter,
		Timeout:           *timeout,
		AuditLogger:       logger,
		RunBaseDir:        filepath.Join(resolved.ArtifactsDir, "runs"),
		FollowTranscripts: *follow,
		FollowLines:       *followLines,
		FollowWriter:      os.Stdout,
	})

	finishPayload := map[string]any{
		"plan":    absPlan,
		"adapter": adapter.Name(),
		"workdir": absWorkDir,
	}
	if res != nil {
		finishPayload["run_id"] = res.RunID
		finishPayload["run_dir"] = res.RunDir
		finishPayload["items_run"] = len(res.ItemRuns)
	}
	if runErr != nil {
		finishPayload["error"] = runErr.Error()
	}
	if err := logger.LogEvent("cli", "plan_run_finished", finishPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	if runErr != nil {
		return runErr
	}
	fmt.Fprintf(os.Stdout, "Plan run complete: %s\n", res.RunDir)
	return nil
}

func runOKRPropose(args []string, workspacePath string) error {
	fs := flag.NewFlagSet("okr propose", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	agentID := fs.String("agent", "", "Agent ID proposing the change")
	updatesDir := fs.String("from", "", "Path to updated OKR YAML files")
	okrsDir := fs.String("okrs-dir", "", "Path to current OKRs (default: <workspace>/okrs)")
	cultureDir := fs.String("culture-dir", "", "Path to culture directory (default: <workspace>/culture)")
	metricsDir := fs.String("metrics-dir", "", "Path to metrics directory (default: <workspace>/metrics)")
	artifactsDir := fs.String("artifacts-dir", "", "Path to artifacts directory (default: <workspace>/artifacts)")
	auditDB := fs.String("audit-db", "", "Path to audit SQLite DB (default: <workspace>/audit/audit.sqlite)")
	proposalsDir := fs.String("proposals-dir", "", "Directory to write proposals (default: <workspace>/artifacts/proposals)")
	note := fs.String("note", "", "Optional proposal note")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *agentID == "" {
		return fmt.Errorf("agent is required")
	}
	if *updatesDir == "" {
		return fmt.Errorf("--from path is required")
	}

	resolved, err := resolveWorkspaceAndOverrides(workspacePath, workspaceOverrides{
		OKRsDir:      *okrsDir,
		CultureDir:   *cultureDir,
		MetricsDir:   *metricsDir,
		ArtifactsDir: *artifactsDir,
		AuditDB:      *auditDB,
	})
	if err != nil {
		return err
	}
	if err := resolved.Workspace.EnsureDirs(); err != nil {
		return err
	}
	absUpdatesDir, err := resolved.Workspace.ResolvePath(*updatesDir)
	if err != nil {
		return fmt.Errorf("resolve --from path: %w", err)
	}
	*okrsDir = resolved.OKRsDir
	if *proposalsDir == "" {
		*proposalsDir = filepath.Join(resolved.ArtifactsDir, "proposals")
	} else {
		*proposalsDir, err = resolved.Workspace.ResolvePath(*proposalsDir)
		if err != nil {
			return fmt.Errorf("resolve --proposals-dir: %w", err)
		}
	}

	logger := audit.NewLogger(resolved.AuditDB)
	startPayload := map[string]any{
		"agent_id":      *agentID,
		"updates_dir":   absUpdatesDir,
		"okrs_dir":      *okrsDir,
		"proposals_dir": *proposalsDir,
	}
	if err := logger.LogEvent(*agentID, "okr_propose_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	meta, err := okrstore.CreateProposal(*agentID, absUpdatesDir, *okrsDir, *proposalsDir, *note)
	finishPayload := map[string]any{
		"agent_id": *agentID,
		"from":     absUpdatesDir,
		"okrs_dir": *okrsDir,
	}

	if err != nil {
		finishPayload["error"] = err.Error()
		_ = logger.LogEvent(*agentID, "okr_propose_finished", finishPayload)
		return err
	}

	finishPayload["proposal_dir"] = meta.ProposalDir
	finishPayload["files"] = meta.Files
	_ = logger.LogEvent(*agentID, "okr_propose_finished", finishPayload)

	fmt.Fprintf(os.Stdout, "Proposal created: %s\n", meta.ProposalDir)
	if len(meta.Files) > 0 {
		fmt.Fprintf(os.Stdout, "Included files: %s\n", strings.Join(meta.Files, ", "))
	}
	if meta.DiffFile != "" {
		fmt.Fprintf(os.Stdout, "Diff: %s\n", filepath.Join(meta.ProposalDir, meta.DiffFile))
	}
	return nil
}

func runOKRApply(args []string, workspacePath string) error {
	fs := flag.NewFlagSet("okr apply", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	proposalPath := fs.String("proposal", "", "Path to proposal directory")
	okrsDir := fs.String("okrs-dir", "", "Path to OKR YAML directory (default: <workspace>/okrs)")
	cultureDir := fs.String("culture-dir", "", "Path to culture directory (default: <workspace>/culture)")
	metricsDir := fs.String("metrics-dir", "", "Path to metrics directory (default: <workspace>/metrics)")
	artifactsDir := fs.String("artifacts-dir", "", "Path to artifacts directory (default: <workspace>/artifacts)")
	auditDB := fs.String("audit-db", "", "Path to audit SQLite DB (default: <workspace>/audit/audit.sqlite)")
	confirm := fs.Bool("i-understand", false, "Explicitly confirm applying OKR changes")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *proposalPath == "" {
		return fmt.Errorf("--proposal path is required")
	}
	if !*confirm {
		return fmt.Errorf("--i-understand flag is required to apply")
	}

	resolved, err := resolveWorkspaceAndOverrides(workspacePath, workspaceOverrides{
		OKRsDir:      *okrsDir,
		CultureDir:   *cultureDir,
		MetricsDir:   *metricsDir,
		ArtifactsDir: *artifactsDir,
		AuditDB:      *auditDB,
	})
	if err != nil {
		return err
	}
	if err := resolved.Workspace.EnsureDirs(); err != nil {
		return err
	}
	absProposalPath, err := resolved.Workspace.ResolvePath(*proposalPath)
	if err != nil {
		return fmt.Errorf("resolve --proposal: %w", err)
	}

	logger := audit.NewLogger(resolved.AuditDB)
	startPayload := map[string]any{
		"proposal": absProposalPath,
	}
	if err := logger.LogEvent("cli", "okr_apply_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	meta, err := okrstore.ApplyProposal(absProposalPath, *confirm)
	finishPayload := map[string]any{
		"proposal": absProposalPath,
	}
	if err != nil {
		finishPayload["error"] = err.Error()
		_ = logger.LogEvent("cli", "okr_apply_finished", finishPayload)
		return err
	}

	finishPayload["okrs_dir"] = meta.OKRsDir
	finishPayload["agent_id"] = meta.AgentID
	_ = logger.LogEvent("cli", "okr_apply_finished", finishPayload)

	fmt.Fprintf(os.Stdout, "Applied proposal %s to %s\n", meta.ID, meta.OKRsDir)
	return nil
}

func runKRMeasure(args []string, workspacePath string) error {
	fs := flag.NewFlagSet("kr measure", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asOfStr := fs.String("as-of", "", "As-of date (YYYY-MM-DD, default: today UTC)")
	repoDir := fs.String("repo-dir", "", "Git repo directory for git metrics (default: <workspace>)")
	metricsDir := fs.String("metrics-dir", "", "Base directory for metric inputs/outputs (default: <workspace>/metrics)")
	okrsDir := fs.String("okrs-dir", "", "Path to OKR YAML directory (default: <workspace>/okrs)")
	cultureDir := fs.String("culture-dir", "", "Path to culture directory (default: <workspace>/culture)")
	artifactsDir := fs.String("artifacts-dir", "", "Path to artifacts directory (default: <workspace>/artifacts)")
	auditDB := fs.String("audit-db", "", "Path to audit SQLite DB (default: <workspace>/audit/audit.sqlite)")
	snapshotsDir := fs.String("snapshots-dir", "", "Directory to write metric snapshots (default: <metrics-dir>/snapshots)")
	ciReport := fs.String("ci-report", "", "Path to CI JSON report (default: <metrics-dir>/ci_report.json)")
	manualPath := fs.String("manual", "", "Path to manual metrics YAML (default: <metrics-dir>/manual.yml)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	resolved, err := resolveWorkspaceAndOverrides(workspacePath, workspaceOverrides{
		OKRsDir:      *okrsDir,
		CultureDir:   *cultureDir,
		MetricsDir:   *metricsDir,
		ArtifactsDir: *artifactsDir,
		AuditDB:      *auditDB,
	})
	if err != nil {
		return err
	}
	if err := resolved.Workspace.EnsureDirs(); err != nil {
		return err
	}
	if *repoDir == "" {
		*repoDir = resolved.Workspace.Root
	} else {
		*repoDir, err = resolved.Workspace.ResolvePath(*repoDir)
		if err != nil {
			return fmt.Errorf("resolve --repo-dir: %w", err)
		}
	}
	*metricsDir = resolved.MetricsDir
	if *snapshotsDir == "" {
		*snapshotsDir = filepath.Join(*metricsDir, "snapshots")
	} else {
		*snapshotsDir, err = resolved.Workspace.ResolvePath(*snapshotsDir)
		if err != nil {
			return fmt.Errorf("resolve --snapshots-dir: %w", err)
		}
	}
	if *ciReport == "" {
		*ciReport = filepath.Join(*metricsDir, "ci_report.json")
	} else {
		*ciReport, err = resolved.Workspace.ResolvePath(*ciReport)
		if err != nil {
			return fmt.Errorf("resolve --ci-report: %w", err)
		}
	}
	if *manualPath == "" {
		*manualPath = filepath.Join(*metricsDir, "manual.yml")
	} else {
		*manualPath, err = resolved.Workspace.ResolvePath(*manualPath)
		if err != nil {
			return fmt.Errorf("resolve --manual: %w", err)
		}
	}

	asOf := time.Now().UTC().Truncate(24 * time.Hour)
	if *asOfStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02", *asOfStr, time.UTC)
		if err != nil {
			return fmt.Errorf("parse --as-of: %w", err)
		}
		asOf = parsed.UTC().Truncate(24 * time.Hour)
	}

	logger := audit.NewLogger(resolved.AuditDB)
	startPayload := map[string]any{
		"workspace":     resolved.Workspace.Root,
		"as_of":         asOf.Format("2006-01-02"),
		"repo_dir":      *repoDir,
		"metrics_dir":   *metricsDir,
		"snapshots_dir": *snapshotsDir,
		"ci_report":     *ciReport,
		"manual_path":   *manualPath,
	}
	if err := logger.LogEvent("cli", "kr_measure_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	providers := []metrics.Provider{
		&metrics.GitProvider{RepoDir: *repoDir, AsOf: asOf},
		&metrics.CIProvider{ReportPath: *ciReport, AsOf: asOf},
		&metrics.ManualProvider{Path: *manualPath, AsOf: asOf},
	}

	ctx := context.Background()
	points, err := metrics.CollectAll(ctx, providers)
	if err != nil {
		finishPayload := map[string]any{
			"error": err.Error(),
		}
		_ = logger.LogEvent("cli", "kr_measure_finished", finishPayload)
		return err
	}

	snapshotPath := metrics.SnapshotPathForDate(*snapshotsDir, asOf)
	snapshot := metrics.Snapshot{
		AsOf:   asOf.Format("2006-01-02"),
		Points: points,
	}
	if err := metrics.WriteSnapshot(snapshotPath, snapshot); err != nil {
		finishPayload := map[string]any{
			"snapshot_path": snapshotPath,
			"error":         err.Error(),
		}
		_ = logger.LogEvent("cli", "kr_measure_finished", finishPayload)
		return err
	}

	finishPayload := map[string]any{
		"snapshot_path": snapshotPath,
		"point_count":   len(points),
	}
	_ = logger.LogEvent("cli", "kr_measure_finished", finishPayload)

	fmt.Fprintf(os.Stdout, "Wrote snapshot: %s\n", snapshotPath)
	return nil
}

func runKRScore(args []string, workspacePath string) error {
	fs := flag.NewFlagSet("kr score", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	okrsDir := fs.String("okrs-dir", "", "Path to OKR YAML directory (default: <workspace>/okrs)")
	cultureDir := fs.String("culture-dir", "", "Path to culture directory (default: <workspace>/culture)")
	metricsDir := fs.String("metrics-dir", "", "Base directory for metric inputs (default: <workspace>/metrics)")
	artifactsDir := fs.String("artifacts-dir", "", "Directory to write score report (default: <workspace>/artifacts)")
	auditDB := fs.String("audit-db", "", "Path to audit SQLite DB (default: <workspace>/audit/audit.sqlite)")
	snapshotsDir := fs.String("snapshots-dir", "", "Directory to read metric snapshots (default: <metrics-dir>/snapshots)")
	snapshotPath := fs.String("snapshot", "", "Path to snapshot JSON (default: latest in snapshots-dir)")
	output := fs.String("output", "", "Output report path (default: <workspace>/artifacts/kr_score_<as-of>.json)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	resolved, err := resolveWorkspaceAndOverrides(workspacePath, workspaceOverrides{
		OKRsDir:      *okrsDir,
		CultureDir:   *cultureDir,
		MetricsDir:   *metricsDir,
		ArtifactsDir: *artifactsDir,
		AuditDB:      *auditDB,
	})
	if err != nil {
		return err
	}
	if err := resolved.Workspace.EnsureDirs(); err != nil {
		return err
	}
	*okrsDir = resolved.OKRsDir
	*metricsDir = resolved.MetricsDir
	*artifactsDir = resolved.ArtifactsDir

	if *snapshotsDir == "" {
		*snapshotsDir = filepath.Join(*metricsDir, "snapshots")
	} else {
		*snapshotsDir, err = resolved.Workspace.ResolvePath(*snapshotsDir)
		if err != nil {
			return fmt.Errorf("resolve --snapshots-dir: %w", err)
		}
	}

	logger := audit.NewLogger(resolved.AuditDB)
	startSnapshot := *snapshotPath
	if startSnapshot == "" {
		startSnapshot = "latest"
	}
	startPayload := map[string]any{
		"workspace":     resolved.Workspace.Root,
		"okrs_dir":      *okrsDir,
		"metrics_dir":   *metricsDir,
		"snapshots_dir": *snapshotsDir,
		"snapshot":      startSnapshot,
	}
	if err := logger.LogEvent("cli", "kr_score_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	path := *snapshotPath
	if path == "" {
		latest, err := metrics.LatestSnapshotPath(*snapshotsDir)
		if err != nil {
			finishPayload := map[string]any{
				"snapshots_dir": *snapshotsDir,
				"error":         err.Error(),
			}
			_ = logger.LogEvent("cli", "kr_score_finished", finishPayload)
			return err
		}
		path = latest
	} else {
		path, err = resolved.Workspace.ResolvePath(path)
		if err != nil {
			finishPayload := map[string]any{
				"snapshot": path,
				"error":    err.Error(),
			}
			_ = logger.LogEvent("cli", "kr_score_finished", finishPayload)
			return fmt.Errorf("resolve --snapshot: %w", err)
		}
	}

	snapshot, err := metrics.LoadSnapshot(path)
	if err != nil {
		finishPayload := map[string]any{
			"snapshot": path,
			"error":    err.Error(),
		}
		_ = logger.LogEvent("cli", "kr_score_finished", finishPayload)
		return err
	}

	store, err := okrstore.LoadFromDir(*okrsDir)
	if err != nil {
		finishPayload := map[string]any{
			"okrs_dir": *okrsDir,
			"error":    err.Error(),
		}
		_ = logger.LogEvent("cli", "kr_score_finished", finishPayload)
		return err
	}

	report, err := metrics.ScoreKRs(store, snapshot, path)
	if err != nil {
		finishPayload := map[string]any{
			"snapshot": path,
			"error":    err.Error(),
		}
		_ = logger.LogEvent("cli", "kr_score_finished", finishPayload)
		return err
	}

	outPath := *output
	if outPath == "" {
		outPath = filepath.Join(*artifactsDir, fmt.Sprintf("kr_score_%s.json", report.AsOf))
	} else {
		outPath, err = resolved.Workspace.ResolvePath(outPath)
		if err != nil {
			return fmt.Errorf("resolve --output: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		finishPayload := map[string]any{
			"output": outPath,
			"error":  err.Error(),
		}
		_ = logger.LogEvent("cli", "kr_score_finished", finishPayload)
		return fmt.Errorf("ensure artifacts dir: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		finishPayload := map[string]any{
			"output": outPath,
			"error":  err.Error(),
		}
		_ = logger.LogEvent("cli", "kr_score_finished", finishPayload)
		return fmt.Errorf("marshal score report: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		finishPayload := map[string]any{
			"output": outPath,
			"error":  err.Error(),
		}
		_ = logger.LogEvent("cli", "kr_score_finished", finishPayload)
		return fmt.Errorf("write score report: %w", err)
	}

	finishPayload := map[string]any{
		"output":  outPath,
		"as_of":   report.AsOf,
		"metrics": len(report.Results),
	}
	_ = logger.LogEvent("cli", "kr_score_finished", finishPayload)

	fmt.Fprintf(os.Stdout, "Wrote score report: %s\n", outPath)
	return nil
}

func writeFileIfMissing(path string, contents string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ensure dir for %s: %w", path, err)
	}
	return os.WriteFile(path, []byte(contents), 0o644)
}

const minimalValuesTemplate = `# Values

- Clarity over ambiguity.
- Evidence over assumptions.
`

const minimalStandardsTemplate = `# Standards

- Keep changes small and reversible.
- Capture evidence for KR claims.
`

const minimalOrgTemplate = `scope: org
objectives:
  - objective_id: OBJ-INIT-1
    objective: Establish a baseline OKR workspace.
    owner_id: team-okr
    key_results:
      - kr_id: KR-INIT-1
        description: Produce a baseline metric snapshot.
        owner_id: team-okr
        metric_key: manual.baseline_snapshot
        baseline: 0
        target: 1
        confidence: 0.5
        status: in_progress
        evidence:
          - init:baseline
`

const minimalPermissionsTemplate = `permissions:
  read:
    - all
  write:
    - owner_id_match
`

const minimalManualMetricsTemplate = `metrics:
  - key: manual.baseline_snapshot
    value: 0
    unit: count
    evidence:
      - init:seed
`

const minimalCIReportTemplate = `{
  "metrics": {
    "pass_rate_30d": 1
  }
}
`

func runDaemon(args []string, workspacePath string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return fmt.Errorf("%s daemon: missing subcommand", appName)
	}

	switch args[0] {
	case "run":
		return runDaemonRun(args[1:], workspacePath)
	case "status":
		return runDaemonStatus(args[1:], workspacePath)
	case "enqueue":
		return runDaemonEnqueue(args[1:], workspacePath)
	default:
		return fmt.Errorf("%s daemon: unknown subcommand %q", appName, args[0])
	}
}

func runDaemonRun(args []string, workspacePath string) error {
	fs := flag.NewFlagSet("daemon run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pollInterval := fs.Duration("poll", 1*time.Second, "Poll interval for checking jobs")
	leaseDuration := fs.Duration("lease", 30*time.Second, "Lease duration for claimed jobs")
	tz := fs.String("tz", "America/Chicago", "Timezone for scheduling")

	if err := fs.Parse(args); err != nil {
		return err
	}

	resolved, err := resolveWorkspaceAndOverrides(workspacePath, workspaceOverrides{})
	if err != nil {
		return err
	}
	if err := resolved.Workspace.EnsureDirs(); err != nil {
		return err
	}

	cfg := daemon.Config{
		Workspace:    resolved.Workspace,
		StorePath:    resolved.Workspace.StateDBPath,
		TimeZone:     *tz,
		PollInterval: *pollInterval,
		LeaseFor:     *leaseDuration,
	}

	d, err := daemon.New(cfg)
	if err != nil {
		return fmt.Errorf("create daemon: %w", err)
	}
	defer d.Close()

	fmt.Fprintf(os.Stdout, "Starting daemon for workspace: %s\n", resolved.Workspace.Root)
	fmt.Fprintf(os.Stdout, "Poll interval: %s, Lease: %s\n", *pollInterval, *leaseDuration)

	ctx := context.Background()
	return d.Run(ctx)
}

func runDaemonStatus(args []string, workspacePath string) error {
	fs := flag.NewFlagSet("daemon status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		return err
	}

	resolved, err := resolveWorkspaceAndOverrides(workspacePath, workspaceOverrides{})
	if err != nil {
		return err
	}

	store, err := daemon.Open(resolved.Workspace.StateDBPath)
	if err != nil {
		return fmt.Errorf("open daemon store: %w", err)
	}
	defer store.Close()

	// Show running jobs
	running, err := store.ListRunning()
	if err != nil {
		return fmt.Errorf("list running jobs: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Running jobs: %d\n", len(running))
	for _, job := range running {
		fmt.Fprintf(os.Stdout, "  %s [%s] started=%s lease_expires=%s\n",
			job.ID, job.Type, job.StartedAt.Format(time.RFC3339), job.LeaseExpiresAt.Format(time.RFC3339))
	}
	fmt.Fprintln(os.Stdout)

	// Show queued jobs (next 10)
	queued, err := store.ListQueued(10)
	if err != nil {
		return fmt.Errorf("list queued jobs: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Queued jobs (next %d):\n", len(queued))
	for _, job := range queued {
		fmt.Fprintf(os.Stdout, "  %s [%s] scheduled=%s\n",
			job.ID, job.Type, job.ScheduledAt.Format(time.RFC3339))
	}
	fmt.Fprintln(os.Stdout)

	// Show recent completed jobs
	completed, err := store.ListRecentCompleted(5)
	if err != nil {
		return fmt.Errorf("list completed jobs: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Recent completed jobs (last %d):\n", len(completed))
	for _, job := range completed {
		var finishedStr string
		if job.FinishedAt != nil {
			finishedStr = job.FinishedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(os.Stdout, "  %s [%s] status=%s finished=%s\n",
			job.ID, job.Type, job.Status, finishedStr)
		if job.ResultJSON != "" {
			fmt.Fprintf(os.Stdout, "    result: %s\n", job.ResultJSON)
		}
	}

	return nil
}

func runDaemonEnqueue(args []string, workspacePath string) error {
	if len(args) == 0 {
		return fmt.Errorf("job type is required")
	}

	jobType := args[0]
	remaining := args[1:]

	fs := flag.NewFlagSet("daemon enqueue", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	atStr := fs.String("at", "", "Scheduled time (YYYY-MM-DDTHH:MM format)")
	payloadJSON := fs.String("payload-json", "{}", "Job payload as JSON")

	if err := fs.Parse(remaining); err != nil {
		return err
	}

	if *atStr == "" {
		return fmt.Errorf("--at is required")
	}

	scheduledAt, err := time.Parse("2006-01-02T15:04", *atStr)
	if err != nil {
		return fmt.Errorf("parse --at: %w", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(*payloadJSON), &payload); err != nil {
		return fmt.Errorf("parse --payload-json: %w", err)
	}

	resolved, err := resolveWorkspaceAndOverrides(workspacePath, workspaceOverrides{})
	if err != nil {
		return err
	}

	store, err := daemon.Open(resolved.Workspace.StateDBPath)
	if err != nil {
		return fmt.Errorf("open daemon store: %w", err)
	}
	defer store.Close()

	jobID, created, err := store.EnqueueUnique(jobType, scheduledAt, payload)
	if err != nil {
		return fmt.Errorf("enqueue job: %w", err)
	}

	if created {
		fmt.Fprintf(os.Stdout, "Enqueued job: %s\n", jobID)
	} else {
		fmt.Fprintf(os.Stdout, "Job already exists: %s\n", jobID)
	}

	return nil
}
