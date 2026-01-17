package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"okrchestra/internal/adapters"
	"okrchestra/internal/audit"
	"okrchestra/internal/okrstore"
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
	case "okr", "kr":
		if err := runOKR(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "plan":
		fmt.Fprintf(os.Stdout, "%s %s: stub command\n", appName, args[0])
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
