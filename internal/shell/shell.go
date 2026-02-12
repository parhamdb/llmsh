package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
	"github.com/parhamdb/llmsh/internal/agent"
	"github.com/parhamdb/llmsh/internal/config"
	"github.com/parhamdb/llmsh/internal/llm"
	"github.com/parhamdb/llmsh/internal/tools"
	"github.com/parhamdb/llmsh/internal/tools/builtin"
)

const systemPrompt = `You are llmsh, a natural language shell that replaces bash. The user types either literal shell commands or natural language, and you execute them immediately.

CRITICAL RULES:
1. ALWAYS use the bash tool to execute commands. Never just describe what you would do — DO it.
2. Be concise. After executing, only add commentary if the output alone isn't clear.
3. Literal commands (ls, pwd, cat file.txt, git status) → run them directly via bash tool.
4. Natural language ("show me big files", "delete all .gif files", "compress these into a tarball") → translate to the correct shell command(s) and execute via bash tool.
5. "run test.sh" or "run ./deploy.sh" → execute it directly.
6. For destructive operations (rm, delete, overwrite), the user is a developer — execute when asked.
7. Chain multiple commands when needed. Use && or run multiple bash calls.
8. Use human-readable output (ls -lh, du -sh, etc.) when relevant.
9. The working directory persists between bash calls (cd works and sticks).
10. You can read/write files directly with read_file and write_file when that's simpler than bash.

ENVIRONMENT:
- Current directory: {{CWD}}
- User: {{USER}}@{{HOST}}
- OS: Linux`

// Shell implements the interactive REPL mode.
type Shell struct {
	provider llm.Provider
	registry *tools.Registry
	output   *agent.OutputRouter
	cfg      *config.Config
	messages []llm.Message
	bash     *builtin.BashTool
	username string
	hostname string
}

// New creates a new interactive shell.
func New(provider llm.Provider, registry *tools.Registry, output *agent.OutputRouter, cfg *config.Config) *Shell {
	// Get the bash tool from registry so we can track cwd
	var bash *builtin.BashTool
	if t := registry.Get("bash"); t != nil {
		bash, _ = t.(*builtin.BashTool)
	}
	if bash == nil {
		bash = &builtin.BashTool{}
		registry.Register(bash)
	}

	username := "user"
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "llmsh"
	}

	return &Shell{
		provider: provider,
		registry: registry,
		output:   output,
		cfg:      cfg,
		bash:     bash,
		username: username,
		hostname: hostname,
	}
}

// Run starts the interactive REPL.
func (s *Shell) Run(ctx context.Context) error {
	histPath := HistoryPath()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          s.buildPrompt(),
		HistoryFile:     histPath,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return fmt.Errorf("readline init failed: %w", err)
	}
	defer rl.Close()

	fmt.Fprintf(s.output.Stderr, "\033[1mllmsh\033[0m — natural language shell\n")
	fmt.Fprintf(s.output.Stderr, "Type commands or natural language. \033[90mexit · clear · history\033[0m\n\n")

	for {
		rl.SetPrompt(s.buildPrompt())

		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				continue
			}
			if err == io.EOF {
				fmt.Fprintln(s.output.Stderr)
				return nil
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch strings.ToLower(line) {
		case "exit", "quit":
			return nil
		case "clear":
			s.messages = nil
			fmt.Fprintln(s.output.Stderr, "Context cleared.")
			continue
		case "history":
			fmt.Fprintf(s.output.Stderr, "History: %s\n", histPath)
			continue
		}

		s.executeCommand(ctx, line)
	}
}

func (s *Shell) executeCommand(ctx context.Context, input string) {
	// Update system prompt with current cwd before each request
	s.provider.SetSystem(s.buildSystemPrompt())

	// Append user message
	s.messages = append(s.messages, llm.Message{
		Role: llm.RoleUser,
		Content: []llm.ContentPart{
			{Type: llm.ContentPartText, Text: input},
		},
	})

	// Run agent with full conversation history
	a := agent.New(s.provider, s.registry, s.output, s.cfg.MaxTurns)
	result, err := a.Run(ctx, s.messages)
	if err != nil {
		fmt.Fprintf(s.output.Stderr, "\033[31mError: %v\033[0m\n", err)
		// Remove failed user message
		s.messages = s.messages[:len(s.messages)-1]
		return
	}

	// Append assistant response to history
	s.messages = append(s.messages, llm.Message{
		Role: llm.RoleAssistant,
		Content: []llm.ContentPart{
			{Type: llm.ContentPartText, Text: result},
		},
	})

	// Keep conversation bounded (last 50 messages)
	if len(s.messages) > 50 {
		s.messages = s.messages[len(s.messages)-50:]
	}

	fmt.Fprintln(s.output.Stderr)
}

func (s *Shell) buildSystemPrompt() string {
	cwd := s.bash.Cwd()
	p := systemPrompt
	p = strings.ReplaceAll(p, "{{CWD}}", cwd)
	p = strings.ReplaceAll(p, "{{USER}}", s.username)
	p = strings.ReplaceAll(p, "{{HOST}}", s.hostname)
	return p
}

func (s *Shell) buildPrompt() string {
	cwd := s.bash.Cwd()

	// Shorten home directory to ~
	if home, err := os.UserHomeDir(); err == nil {
		if cwd == home {
			cwd = "~"
		} else if strings.HasPrefix(cwd, home+"/") {
			cwd = "~" + cwd[len(home):]
		}
	}

	// Shorten long paths
	parts := strings.Split(cwd, "/")
	if len(parts) > 3 && !strings.HasPrefix(cwd, "~") {
		cwd = "…/" + filepath.Join(parts[len(parts)-2], parts[len(parts)-1])
	}

	return fmt.Sprintf("\033[32m%s\033[0m:\033[34m%s\033[0m$ ", s.username, cwd)
}
