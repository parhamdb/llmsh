# Changelog

## v26021203 (2026-02-12)
- Config system overhaul: 9-layer precedence (defaults → system → user → credentials → project → .env → frontmatter → env vars → CLI flags)
- Add nested `providers:` block in config and separate `credentials.yaml` file
- Add OpenRouter as 4th provider (reuses OpenAI SDK with custom base URL)
- Add model fallback lists with compact `provider:model` prefix syntax (e.g. `openrouter:qwen/qwen-2.5-72b`)
- Add auto-detection of provider from model name (`claude-*` → anthropic, `gpt-*`/`o1-*`/`o3-*`/`o4-*` → openai)
- Add XDG Base Directory support (`$XDG_CONFIG_HOME/llmsh/` with `~/.llmsh/` fallback)
- Add project-local config (`.llmsh/config.yaml` in CWD)
- Add `.env` and `.env.local` file support (hand-rolled parser, no new deps)
- Add `--config` flag for custom config file path
- Add `--show-config` flag to display merged config with redacted API keys
- Add `LLMSH_TEMPERATURE`, `OPENROUTER_API_KEY` environment variable support

## v26021202 (2026-02-12)
- Fix .gitignore excluding cmd/llmsh and add main.go
- Initial implementation of llmsh — LLM-powered natural language shell

