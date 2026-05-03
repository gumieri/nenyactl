package agents

type ProviderDef struct {
	Name     string
	Help     string
	AuthHint string
	NeedsKey bool
}

type ModelDef struct {
	ID       string
	Provider string
	MaxCtx   int
}

var Providers = []ProviderDef{
	{Name: "gemini", Help: "Google AI", AuthHint: "AIza...", NeedsKey: true},
	{Name: "deepseek", Help: "DeepSeek", AuthHint: "sk-...", NeedsKey: true},
	{Name: "zai", Help: "Z.AI (GLM)", AuthHint: "sk-...", NeedsKey: true},
	{Name: "groq", Help: "Groq", AuthHint: "gsk_...", NeedsKey: true},
	{Name: "together", Help: "Together AI", AuthHint: "sk-...", NeedsKey: true},
	{Name: "anthropic", Help: "Anthropic", AuthHint: "sk-ant-...", NeedsKey: true},
	{Name: "mistral", Help: "Mistral AI", AuthHint: "sk-...", NeedsKey: true},
	{Name: "xai", Help: "xAI (Grok)", AuthHint: "sk-...", NeedsKey: true},
	{Name: "perplexity", Help: "Perplexity", AuthHint: "pplx-...", NeedsKey: true},
	{Name: "cohere", Help: "Cohere", AuthHint: "sk-...", NeedsKey: true},
	{Name: "deepinfra", Help: "DeepInfra", AuthHint: "sk-...", NeedsKey: true},
	{Name: "openrouter", Help: "OpenRouter", AuthHint: "sk-or-...", NeedsKey: true},
	{Name: "nvidia", Help: "NVIDIA", AuthHint: "nvapi-...", NeedsKey: true},
	{Name: "sambanova", Help: "SambaNova", AuthHint: "sk-...", NeedsKey: true},
	{Name: "cerebras", Help: "Cerebras", AuthHint: "sk-...", NeedsKey: true},
	{Name: "github", Help: "GitHub Models", AuthHint: "ghp_...", NeedsKey: true},
	{Name: "zen", Help: "Zen (OpenCode)", AuthHint: "sk-...", NeedsKey: true},
	{Name: "ollama", Help: "Ollama (local)", AuthHint: "none", NeedsKey: false},
}

var Models = []ModelDef{
	// Gemini
	{ID: "gemini-3.1-flash-lite-preview", Provider: "gemini", MaxCtx: 128000},
	{ID: "gemini-3-flash-preview", Provider: "gemini", MaxCtx: 128000},
	{ID: "gemini-2.5-flash-lite", Provider: "gemini", MaxCtx: 128000},
	{ID: "gemini-2.5-flash", Provider: "gemini", MaxCtx: 128000},

	// DeepSeek
	{ID: "deepseek-v4-pro", Provider: "deepseek", MaxCtx: 1000000},
	{ID: "deepseek-v4-flash", Provider: "deepseek", MaxCtx: 1000000},
	{ID: "deepseek-chat", Provider: "deepseek", MaxCtx: 64000},

	// Anthropic
	{ID: "claude-opus-4-7", Provider: "anthropic", MaxCtx: 200000},
	{ID: "claude-opus-4-6", Provider: "anthropic", MaxCtx: 200000},
	{ID: "claude-opus-4-5", Provider: "anthropic", MaxCtx: 200000},
	{ID: "claude-opus-4-0", Provider: "anthropic", MaxCtx: 200000},
	{ID: "claude-sonnet-4-6", Provider: "anthropic", MaxCtx: 200000},
	{ID: "claude-sonnet-4-5", Provider: "anthropic", MaxCtx: 200000},
	{ID: "claude-sonnet-4-0", Provider: "anthropic", MaxCtx: 200000},
	{ID: "claude-sonnet-3-7-20250219", Provider: "anthropic", MaxCtx: 200000},
	{ID: "claude-sonnet-3-5-20241022", Provider: "anthropic", MaxCtx: 200000},
	{ID: "claude-haiku-4-5", Provider: "anthropic", MaxCtx: 200000},
	{ID: "claude-3-5-haiku-20241022", Provider: "anthropic", MaxCtx: 200000},

	// Mistral
	{ID: "mistral-large-latest", Provider: "mistral", MaxCtx: 256000},
	{ID: "mistral-small-latest", Provider: "mistral", MaxCtx: 256000},
	{ID: "mistral-medium-latest", Provider: "mistral", MaxCtx: 256000},
	{ID: "codestral-latest", Provider: "mistral", MaxCtx: 128000},
	{ID: "devstral-medium-latest", Provider: "mistral", MaxCtx: 256000},
	{ID: "magistral-medium-latest", Provider: "mistral", MaxCtx: 128000},
	{ID: "pixtral-large-latest", Provider: "mistral", MaxCtx: 128000},

	// Groq
	{ID: "llama-3.3-70b-versatile", Provider: "groq", MaxCtx: 131072},
	{ID: "mixtral-8x7b-32768", Provider: "groq", MaxCtx: 32768},
	{ID: "llama-3.1-8b-instruct", Provider: "groq", MaxCtx: 8192},

	// xAI (Grok)
	{ID: "grok-4", Provider: "xai", MaxCtx: 256000},
	{ID: "grok-4-fast", Provider: "xai", MaxCtx: 2000000},
	{ID: "grok-3", Provider: "xai", MaxCtx: 131072},
	{ID: "grok-3-fast", Provider: "xai", MaxCtx: 131072},
	{ID: "grok-3-mini", Provider: "xai", MaxCtx: 131072},

	// Perplexity
	{ID: "sonar-pro", Provider: "perplexity", MaxCtx: 200000},
	{ID: "sonar-reasoning-pro", Provider: "perplexity", MaxCtx: 128000},
	{ID: "sonar-deep-research", Provider: "perplexity", MaxCtx: 128000},
	{ID: "sonar", Provider: "perplexity", MaxCtx: 128000},

	// Together
	{ID: "qwen2.5-72b-turbo", Provider: "together", MaxCtx: 32768},
	{ID: "llama-3.1-405b-instruct", Provider: "together", MaxCtx: 128000},

	// Zen
	{ID: "qwen3.5-plus", Provider: "zen", MaxCtx: 131072},
	{ID: "minimax-m2.7", Provider: "zen", MaxCtx: 200000},
	{ID: "kimi-k2.6", Provider: "zen", MaxCtx: 200000},
	{ID: "big-pickle", Provider: "zen", MaxCtx: 200000},

	// NVIDIA
	{ID: "nemotron-3-super", Provider: "nvidia", MaxCtx: 4000},

	// Cerebras
	{ID: "llama-3.3-70b", Provider: "cerebras", MaxCtx: 8192},

	// Sambanova
	{ID: "llama-3.1-405b-instruct", Provider: "sambanova", MaxCtx: 128000},

	// Z.AI
	{ID: "glm-5-turbo", Provider: "zai", MaxCtx: 200000},
	{ID: "glm-4.7-flash", Provider: "zai", MaxCtx: 200000},
}

var DefaultAgents = map[string]struct {
	Strategy string
	Models   []string
}{
	"small": {
		Strategy: "round-robin",
		Models:   []string{"gemini-2.5-flash-lite", "claude-3-5-haiku-latest", "deepseek-v4-flash"},
	},
	"build": {
		Strategy: "fallback",
		Models:   []string{"claude-sonnet-4-5", "gemini-2.5-flash"},
	},
	"plan": {
		Strategy: "fallback",
		Models:   []string{"deepseek-v4-pro", "gemini-3.1-flash-lite-preview"},
	},
}

var Strategies = []string{"fallback", "round-robin"}