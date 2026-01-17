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
	"okrchestra/internal/metrics"
	"okrchestra/internal/okrstore"
	"okrchestra/internal/planner"
)

const appName = "okrchestra"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s: OKR-driven agent orchestration\n\n", appName)
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [command] [flags]\n\n", appName)
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  agent   Manage agents")
		fmt.Fprintln(os.Stderr, "  okr     Manage OKRs")
		fmt.Fprintln(os.Stderr, "  kr      Manage key results")
		fmt.Fprintln(os.Stderr, "  plan    Manage plans")
		fmt.Fprintln(os.Stderr, "  help    Show this help")
		fmt.Fprintln(os.Stderr, "\nFlags:")
		flag.PrintDefaults()
	}

	flag.Parse()

	args := flag.Args()
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		flag.Usage()
		return
	}

	switch args[0] {
	case "agent":
		if err := runAgent(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "okr":
		if err := runOKR(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "kr":
		if err := runKR(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "plan":
		if err := runPlan(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
		flag.Usage()
		os.Exit(1)
	}
}

func runAgent(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return fmt.Errorf("%s agent: missing subcommand", appName)
	}

	switch args[0] {
	case "run":
		return runAgentRun(args[1:])
	default:
		return fmt.Errorf("%s agent: unknown subcommand %q", appName, args[0])
	}
}

func runAgentRun(args []string) error {
	fs := flag.NewFlagSet("agent run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	adapterName := fs.String("adapter", "codex", "Adapter name")
	promptPath := fs.String("prompt", "", "Path to prompt file")
	workDir := fs.String("workdir", ".", "Working directory")
	artifactsDir := fs.String("artifacts", "", "Artifacts directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *promptPath == "" {
		return fmt.Errorf("prompt is required")
	}
	if *artifactsDir == "" {
		return fmt.Errorf("artifacts dir is required")
	}

	absPrompt, err := filepath.Abs(*promptPath)
	if err != nil {
		return fmt.Errorf("resolve prompt path: %w", err)
	}
	absWorkDir, err := filepath.Abs(*workDir)
	if err != nil {
		return fmt.Errorf("resolve workdir: %w", err)
	}
	absArtifactsDir, err := filepath.Abs(*artifactsDir)
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

	startPayload := map[string]any{
		"adapter":   adapter.Name(),
		"prompt":    absPrompt,
		"workdir":   absWorkDir,
		"artifacts": absArtifactsDir,
	}
	if err := audit.LogEvent("cli", "agent_run_started", startPayload); err != nil {
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
	if err := audit.LogEvent("cli", "agent_run_finished", finishPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	return runErr
}

func runOKR(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return fmt.Errorf("%s okr: missing subcommand", appName)
	}

	switch args[0] {
	case "propose":
		return runOKRPropose(args[1:])
	case "apply":
		return runOKRApply(args[1:])
	default:
		return fmt.Errorf("%s okr: unknown subcommand %q", appName, args[0])
	}
}

func runKR(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return fmt.Errorf("%s kr: missing subcommand", appName)
	}

	switch args[0] {
	case "measure":
		return runKRMeasure(args[1:])
	case "score":
		return runKRScore(args[1:])
	default:
		return fmt.Errorf("%s kr: unknown subcommand %q", appName, args[0])
	}
}

func runPlan(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return fmt.Errorf("%s plan: missing subcommand", appName)
	}

	switch args[0] {
	case "generate":
		return runPlanGenerate(args[1:])
	case "run":
		return runPlanRun(args[1:])
	default:
		return fmt.Errorf("%s plan: unknown subcommand %q", appName, args[0])
	}
}

func runPlanGenerate(args []string) error {
	fs := flag.NewFlagSet("plan generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	okrsDir := fs.String("okrs-dir", "okrs", "Path to OKR YAML directory")
	outDir := fs.String("out-dir", filepath.Join("artifacts", "plans"), "Base directory to write plans")
	asOfStr := fs.String("as-of", "", "As-of date (YYYY-MM-DD, default: today UTC)")
	objectiveID := fs.String("objective-id", "", "Optional objective_id to target")
	krID := fs.String("kr-id", "", "Optional kr_id to target")
	agentRole := fs.String("agent-role", "software_engineer", "Agent role for generated items")

	if err := fs.Parse(args); err != nil {
		return err
	}

	asOf := time.Now().UTC().Truncate(24 * time.Hour)
	if *asOfStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02", *asOfStr, time.UTC)
		if err != nil {
			return fmt.Errorf("parse --as-of: %w", err)
		}
		asOf = parsed.UTC().Truncate(24 * time.Hour)
	}

	startPayload := map[string]any{
		"okrs_dir":     *okrsDir,
		"out_dir":      *outDir,
		"as_of":        asOf.Format("2006-01-02"),
		"objective_id": *objectiveID,
		"kr_id":        *krID,
		"agent_role":   *agentRole,
		"command":      "plan generate",
	}
	if err := audit.LogEvent("cli", "plan_generate_started", startPayload); err != nil {
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
		_ = audit.LogEvent("cli", "plan_generate_finished", finishPayload)
		return err
	}

	finishPayload["plan_path"] = res.PlanPath
	finishPayload["plan_id"] = res.Plan.ID
	_ = audit.LogEvent("cli", "plan_generate_finished", finishPayload)

	fmt.Fprintf(os.Stdout, "Wrote plan: %s\n", res.PlanPath)
	return nil
}

func runPlanRun(args []string) error {
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
	workDir := fs.String("workdir", ".", "Working directory")
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

	absPlan, err := filepath.Abs(planArg)
	if err != nil {
		return fmt.Errorf("resolve plan path: %w", err)
	}
	absWorkDir, err := filepath.Abs(*workDir)
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

	startPayload := map[string]any{
		"plan":    absPlan,
		"adapter": adapter.Name(),
		"workdir": absWorkDir,
		"timeout": timeout.String(),
	}
	if err := audit.LogEvent("cli", "plan_run_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	ctx := context.Background()
	res, runErr := planner.RunPlan(ctx, planner.RunOptions{
		PlanPath:          absPlan,
		WorkDir:           absWorkDir,
		Adapter:           adapter,
		Timeout:           *timeout,
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
	if err := audit.LogEvent("cli", "plan_run_finished", finishPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	if runErr != nil {
		return runErr
	}
	fmt.Fprintf(os.Stdout, "Plan run complete: %s\n", res.RunDir)
	return nil
}

func runOKRPropose(args []string) error {
	fs := flag.NewFlagSet("okr propose", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	agentID := fs.String("agent", "", "Agent ID proposing the change")
	updatesDir := fs.String("from", "", "Path to updated OKR YAML files")
	okrsDir := fs.String("okrs-dir", "okrs", "Path to current OKRs")
	proposalsDir := fs.String("proposals-dir", filepath.Join("artifacts", "proposals"), "Directory to write proposals")
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

	startPayload := map[string]any{
		"agent_id":      *agentID,
		"updates_dir":   *updatesDir,
		"okrs_dir":      *okrsDir,
		"proposals_dir": *proposalsDir,
	}
	if err := audit.LogEvent(*agentID, "okr_propose_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	meta, err := okrstore.CreateProposal(*agentID, *updatesDir, *okrsDir, *proposalsDir, *note)
	finishPayload := map[string]any{
		"agent_id": *agentID,
		"from":     *updatesDir,
		"okrs_dir": *okrsDir,
	}

	if err != nil {
		finishPayload["error"] = err.Error()
		_ = audit.LogEvent(*agentID, "okr_propose_finished", finishPayload)
		return err
	}

	finishPayload["proposal_dir"] = meta.ProposalDir
	finishPayload["files"] = meta.Files
	_ = audit.LogEvent(*agentID, "okr_propose_finished", finishPayload)

	fmt.Fprintf(os.Stdout, "Proposal created: %s\n", meta.ProposalDir)
	if len(meta.Files) > 0 {
		fmt.Fprintf(os.Stdout, "Included files: %s\n", strings.Join(meta.Files, ", "))
	}
	if meta.DiffFile != "" {
		fmt.Fprintf(os.Stdout, "Diff: %s\n", filepath.Join(meta.ProposalDir, meta.DiffFile))
	}
	return nil
}

func runOKRApply(args []string) error {
	fs := flag.NewFlagSet("okr apply", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	proposalPath := fs.String("proposal", "", "Path to proposal directory")
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

	startPayload := map[string]any{
		"proposal": *proposalPath,
	}
	if err := audit.LogEvent("cli", "okr_apply_started", startPayload); err != nil {
		fmt.Fprintln(os.Stderr, "audit log failed:", err)
	}

	meta, err := okrstore.ApplyProposal(*proposalPath, *confirm)
	finishPayload := map[string]any{
		"proposal": *proposalPath,
	}
	if err != nil {
		finishPayload["error"] = err.Error()
		_ = audit.LogEvent("cli", "okr_apply_finished", finishPayload)
		return err
	}

	finishPayload["okrs_dir"] = meta.OKRsDir
	finishPayload["agent_id"] = meta.AgentID
	_ = audit.LogEvent("cli", "okr_apply_finished", finishPayload)

	fmt.Fprintf(os.Stdout, "Applied proposal %s to %s\n", meta.ID, meta.OKRsDir)
	return nil
}

func runKRMeasure(args []string) error {
	fs := flag.NewFlagSet("kr measure", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asOfStr := fs.String("as-of", "", "As-of date (YYYY-MM-DD, default: today UTC)")
	repoDir := fs.String("repo-dir", ".", "Git repo directory for git metrics")
	snapshotsDir := fs.String("snapshots-dir", filepath.Join("metrics", "snapshots"), "Directory to write metric snapshots")
	ciReport := fs.String("ci-report", filepath.Join("metrics", "ci_report.json"), "Path to CI JSON report")
	manualPath := fs.String("manual", filepath.Join("metrics", "manual.yml"), "Path to manual metrics YAML")

	if err := fs.Parse(args); err != nil {
		return err
	}

	asOf := time.Now().UTC().Truncate(24 * time.Hour)
	if *asOfStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02", *asOfStr, time.UTC)
		if err != nil {
			return fmt.Errorf("parse --as-of: %w", err)
		}
		asOf = parsed.UTC().Truncate(24 * time.Hour)
	}

	providers := []metrics.Provider{
		&metrics.GitProvider{RepoDir: *repoDir, AsOf: asOf},
		&metrics.CIProvider{ReportPath: *ciReport, AsOf: asOf},
		&metrics.ManualProvider{Path: *manualPath, AsOf: asOf},
	}

	ctx := context.Background()
	points, err := metrics.CollectAll(ctx, providers)
	if err != nil {
		return err
	}

	snapshotPath := metrics.SnapshotPathForDate(*snapshotsDir, asOf)
	snapshot := metrics.Snapshot{
		AsOf:   asOf.Format("2006-01-02"),
		Points: points,
	}
	if err := metrics.WriteSnapshot(snapshotPath, snapshot); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "Wrote snapshot: %s\n", snapshotPath)
	return nil
}

func runKRScore(args []string) error {
	fs := flag.NewFlagSet("kr score", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	okrsDir := fs.String("okrs-dir", "okrs", "Path to OKR YAML directory")
	snapshotsDir := fs.String("snapshots-dir", filepath.Join("metrics", "snapshots"), "Directory to read metric snapshots")
	snapshotPath := fs.String("snapshot", "", "Path to snapshot JSON (default: latest in snapshots-dir)")
	artifactsDir := fs.String("artifacts-dir", "artifacts", "Directory to write score report")
	output := fs.String("output", "", "Output report path (default: artifacts/kr_score_<as-of>.json)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	path := *snapshotPath
	if path == "" {
		latest, err := metrics.LatestSnapshotPath(*snapshotsDir)
		if err != nil {
			return err
		}
		path = latest
	}

	snapshot, err := metrics.LoadSnapshot(path)
	if err != nil {
		return err
	}

	store, err := okrstore.LoadFromDir(*okrsDir)
	if err != nil {
		return err
	}

	report, err := metrics.ScoreKRs(store, snapshot, path)
	if err != nil {
		return err
	}

	outPath := *output
	if outPath == "" {
		outPath = filepath.Join(*artifactsDir, fmt.Sprintf("kr_score_%s.json", report.AsOf))
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("ensure artifacts dir: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal score report: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("write score report: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Wrote score report: %s\n", outPath)
	return nil
}
