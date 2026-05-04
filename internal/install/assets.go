package install

const ExampleConfig = `{
  // Nenya Configuration
  "server": {
    "listen_addr": ":8080",
    "max_body_bytes": 10485760
  },

  // Context management: truncation strategies and TF-IDF relevance scoring
  "context": {
    "truncation_strategy": "middle-out",
    "truncation_keep_first_pct": 15.0,
    "truncation_keep_last_pct": 25.0
  },

  // Governance: rate limiting, routing, and request lifecycle
  "governance": {
    "ratelimit_max_tpm": 250000,
    "ratelimit_max_rpm": 15
  },

  // Bouncer: LLM-based privacy filter for sensitive data redaction
  "bouncer": {
    "enabled": true,
    "redaction_label": "[REDACTED]",
    "fail_open": true,
    "engine": {
      "provider": "ollama",
      "model": "qwen2.5-coder:7b",
      "system_prompt_file": "./prompts/privacy_filter.md",
      "timeout_seconds": 60
    }
  },

  // Prefix cache alignment for better upstream cache hits
  "prefix_cache": {
    "enabled": true,
    "pin_system_first": true,
    "stable_tools": true,
    "skip_redaction_on_system": true
  },

  // Compaction presets: "aggressive", "balanced", or "minimal"
  "compaction": {
    "compaction_preset": "balanced"
  },

  // Window: sliding context window with summarization
  "window": {
    "enabled": false,
    "mode": "summarize",
    "active_messages": 6,
    "trigger_ratio": 0.8,
    "summary_max_runes": 4000,
    "max_context": 128000,
    "engine": {
      "provider": "ollama",
      "model": "qwen2.5-coder:7b",
      "system_prompt_file": "./prompts/summarizer.md",
      "timeout_seconds": 60
    }
  },

  // Response cache: LRU cache for upstream completions
  "response_cache": {
    "enabled": false,
    "max_entries": 512,
    "max_entry_bytes": 1048576,
    "ttl_seconds": 3600,
    "evict_every_seconds": 300,
    "force_refresh_header": "x-nenya-cache-force-refresh"
  },

  // Agents: named model groups with fallback chains
  "agents": {
    "build": {
      "strategy": "fallback",
      "cooldown_seconds": 60,
      "failure_threshold": 5,
      "models": [
        "gemini-2.5-flash",
        "deepseek-chat"
      ]
    }
  },

  // Provider endpoint configurations
  "providers": {
    "gemini": {
      "url": "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
      "auth_style": "bearer+x-goog"
    },
    "deepseek": {
      "url": "https://api.deepseek.com/chat/completions",
      "auth_style": "bearer"
    },
    "ollama": {
      "url": "http://127.0.0.1:11434/v1/chat/completions",
      "auth_style": "none"
    }
  }
}
`

const SecretsExample = `{
  "client_token": "nk-",
  "provider_keys": {
    "gemini": "AIza...",
    "deepseek": "sk-..."
  }
}
`
