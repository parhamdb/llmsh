package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/parhamdb/llmsh/internal/agent"
	"github.com/parhamdb/llmsh/internal/builder"
	"github.com/parhamdb/llmsh/internal/config"
	"github.com/parhamdb/llmsh/internal/llm"
	anthropicProvider "github.com/parhamdb/llmsh/internal/llm/anthropic"
	ollamaProvider "github.com/parhamdb/llmsh/internal/llm/ollama"
	openaiProvider "github.com/parhamdb/llmsh/internal/llm/openai"
	openrouterProvider "github.com/parhamdb/llmsh/internal/llm/openrouter"
	"github.com/parhamdb/llmsh/internal/parser"
	"github.com/parhamdb/llmsh/internal/shell"
	"github.com/parhamdb/llmsh/internal/template"
	"github.com/parhamdb/llmsh/internal/tools"
	"github.com/parhamdb/llmsh/internal/tools/builtin"
)

var version = "dev"

func main() {
	// CLI flags
	versionFlag := flag.Bool("version", false, "Print version")
	modelFlag := flag.String("model", "", "LLM model to use")
	providerFlag := flag.String("provider", "", "LLM provider (anthropic, openai, ollama, openrouter)")
	tempFlag := flag.Float64("temp", -1, "Temperature (-1 = use default)")
	maxTurnsFlag := flag.Int("max-turns", 0, "Maximum agent turns")
	verboseFlag := flag.Bool("verbose", false, "Verbose output")
	quietFlag := flag.Bool("quiet", false, "Quiet mode (only final output)")
	dryRunFlag := flag.Bool("dry-run", false, "Parse and show config without executing")
	showConfigFlag := flag.Bool("show-config", false, "Show merged config and exit")
	configFileFlag := flag.String("config", "", "Path to config file")
	newFlag := flag.String("new", "", "Create a new .llmsh script (optionally specify output filename)")
	editFlag := flag.String("edit", "", "Edit an existing .llmsh script")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("llmsh v%s\n", version)
		os.Exit(0)
	}

	// Build CLI config
	cli := config.CLIConfig{
		Verbose:    *verboseFlag,
		Quiet:      *quietFlag,
		DryRun:     *dryRunFlag,
		ShowConfig: *showConfigFlag,
		ConfigFile: *configFileFlag,
	}
	if *modelFlag != "" {
		cli.Model = modelFlag
	}
	if *providerFlag != "" {
		cli.Provider = providerFlag
	}
	if *tempFlag >= 0 {
		cli.Temperature = tempFlag
	}
	if *maxTurnsFlag > 0 {
		cli.MaxTurns = maxTurnsFlag
	}

	// Handle --show-config before mode switch
	if *showConfigFlag {
		cfg, err := config.Load(cli, parser.Frontmatter{})
		if err != nil {
			fatal("Error loading config: %v", err)
		}
		fmt.Fprint(os.Stderr, config.ShowConfig(cfg))
		os.Exit(0)
	}

	// Context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Mode routing
	args := flag.Args()

	switch {
	case *newFlag != "" || (len(args) == 0 && flagPresent("new")):
		// --new mode
		runNew(ctx, cli, *newFlag)
	case *editFlag != "":
		// --edit mode
		runEdit(ctx, cli, *editFlag)
	case len(args) == 0:
		// Interactive shell mode
		runShell(ctx, cli)
	default:
		// Script mode
		runScript(ctx, cli, args[0], args[1:])
	}
}

func flagPresent(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func runScript(ctx context.Context, cli config.CLIConfig, scriptPath string, scriptArgs []string) {
	// Parse script
	f, err := os.Open(scriptPath)
	if err != nil {
		fatal("Error opening script: %v", err)
	}
	defer f.Close()

	script, err := parser.Parse(f)
	if err != nil {
		fatal("Error parsing script: %v", err)
	}

	// Load config
	cfg, err := config.Load(cli, script.Frontmatter)
	if err != nil {
		fatal("Error loading config: %v", err)
	}

	// Render template
	rendered, err := template.Render(script.Body, script.Frontmatter.Args, scriptArgs)
	if err != nil {
		fatal("Error rendering template: %v", err)
	}
	cfg.System = rendered

	if cli.DryRun {
		fmt.Fprint(os.Stderr, config.ShowConfig(cfg))
		fmt.Fprintf(os.Stderr, "\n--- System Prompt ---\n%s\n", cfg.System)
		return
	}

	// Setup tools
	registry := buildRegistry(cfg.Tools)

	// Apply permission guard
	executor := tools.Executor(registry)
	if len(cfg.Permissions) > 0 {
		executor = tools.NewPermissionGuard(registry, cfg.Permissions)
	}

	// Create provider
	provider := createProvider(cfg)

	// Create output router
	output := agent.NewOutputRouter(cli.Verbose, cli.Quiet)

	// Build initial messages
	messages := []llm.Message{
		{
			Role: llm.RoleUser,
			Content: []llm.ContentPart{
				{Type: llm.ContentPartText, Text: "Execute the task described in your system prompt."},
			},
		},
	}

	// Run agent
	a := agent.New(provider, executor, output, cfg.MaxTurns)
	result, err := a.Run(ctx, messages)
	if err != nil {
		fatal("Agent error: %v", err)
	}

	// Output final result to stdout (for piping)
	if !cli.Quiet {
		// Streaming already showed the output on stderr
		// Only write to stdout if quiet mode or for piping
	} else {
		output.FinalResult(result)
	}
}

func runShell(ctx context.Context, cli config.CLIConfig) {
	cfg, err := config.Load(cli, parser.Frontmatter{})
	if err != nil {
		fatal("Error loading config: %v", err)
	}

	// Shell mode always gets all tools
	allTools := []string{"bash", "read_file", "write_file", "list_files", "http_request", "env"}
	registry := buildRegistry(allTools)
	provider := createProvider(cfg)
	output := agent.NewOutputRouter(cli.Verbose, false) // never quiet in shell mode

	sh := shell.New(provider, registry, output, cfg)
	if err := sh.Run(ctx); err != nil {
		fatal("Shell error: %v", err)
	}
}

func runNew(ctx context.Context, cli config.CLIConfig, outputFile string) {
	cfg, err := config.Load(cli, parser.Frontmatter{})
	if err != nil {
		fatal("Error loading config: %v", err)
	}

	provider := createProvider(cfg)
	b := builder.NewBuilder(provider)
	if err := b.Run(ctx, outputFile); err != nil {
		fatal("Builder error: %v", err)
	}
}

func runEdit(ctx context.Context, cli config.CLIConfig, filePath string) {
	cfg, err := config.Load(cli, parser.Frontmatter{})
	if err != nil {
		fatal("Error loading config: %v", err)
	}

	provider := createProvider(cfg)
	e := builder.NewEditor(provider)
	if err := e.Run(ctx, filePath); err != nil {
		fatal("Editor error: %v", err)
	}
}

func buildRegistry(enabledTools []string) *tools.Registry {
	// All available tools
	allTools := map[string]tools.Tool{
		"bash":         &builtin.BashTool{},
		"read_file":    &builtin.ReadFileTool{},
		"write_file":   &builtin.WriteFileTool{},
		"list_files":   &builtin.ListFilesTool{},
		"http_request": &builtin.HTTPRequestTool{},
		"env":          &builtin.EnvTool{},
	}

	registry := tools.NewRegistry()
	enabled := make(map[string]bool)
	for _, t := range enabledTools {
		enabled[t] = true
	}

	for name, tool := range allTools {
		if enabled[name] {
			registry.Register(tool)
		}
	}

	return registry
}

func createProvider(cfg *config.Config) llm.Provider {
	switch cfg.Provider {
	case "anthropic":
		opts := []anthropicProvider.Option{
			anthropicProvider.WithModel(cfg.Model),
			anthropicProvider.WithTemperature(cfg.Temperature),
		}
		if cfg.System != "" {
			opts = append(opts, anthropicProvider.WithSystem(cfg.System))
		}
		return anthropicProvider.New(cfg.Providers.Anthropic.APIKey, opts...)

	case "openai":
		opts := []openaiProvider.Option{
			openaiProvider.WithModel(cfg.Model),
			openaiProvider.WithTemperature(cfg.Temperature),
		}
		if cfg.System != "" {
			opts = append(opts, openaiProvider.WithSystem(cfg.System))
		}
		return openaiProvider.New(cfg.Providers.OpenAI.APIKey, opts...)

	case "ollama":
		opts := []openaiProvider.Option{
			openaiProvider.WithTemperature(cfg.Temperature),
		}
		if cfg.System != "" {
			opts = append(opts, openaiProvider.WithSystem(cfg.System))
		}
		return ollamaProvider.New(cfg.Providers.Ollama.Host, cfg.Model, opts...)

	case "openrouter":
		opts := []openaiProvider.Option{
			openaiProvider.WithTemperature(cfg.Temperature),
		}
		if cfg.System != "" {
			opts = append(opts, openaiProvider.WithSystem(cfg.System))
		}
		return openrouterProvider.New(cfg.Providers.OpenRouter.APIKey, cfg.Model, opts...)

	default:
		fatal("Unknown provider: %s", cfg.Provider)
		return nil
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
