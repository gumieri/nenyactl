package install

const SystemdService = `# Nenya AI Gateway - systemd service unit
#
# Managed by nenyactl. See: https://github.com/gumieri/nenyactl

[Unit]
Description=Nenya AI Gateway & Bouncer
Requires=nenya.socket
After=nenya.socket

[Service]
Type=simple
DynamicUser=yes
ExecStart=/usr/local/bin/nenya
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s

NoNewPrivileges=yes
LockPersonality=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
RestrictNamespaces=yes

PrivateTmp=yes
PrivateDevices=yes
ProtectSystem=strict
ProtectHome=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
ProtectProc=invisible
RemoveIPC=yes
UMask=0077

RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
MemoryDenyWriteExecute=yes
LimitMEMLOCK=infinity

LoadCredential=secrets:/etc/nenya/secrets.json

[Install]
WantedBy=multi-user.target
`

const SystemdSocket = `# Nenya AI Gateway - systemd socket unit
#
# Managed by nenyactl. See: https://github.com/gumieri/nenyactl

[Unit]
Description=Nenya AI Gateway Socket

[Socket]
ListenStream=8080
Accept=no

[Install]
WantedBy=sockets.target
`

const ExampleConfig = `{
  "server": {
    "listen_addr": ":8080",
    "max_body_bytes": 10485760
  },
  "governance": {
    "ratelimit_max_tpm": 250000,
    "ratelimit_max_rpm": 15,
    "truncation_strategy": "middle-out",
    "keep_first_percent": 15.0,
    "keep_last_percent": 25.0
  },
  "security_filter": {
    "enabled": true,
    "skip_on_engine_failure": true,
    "engine": {
      "provider": "ollama",
      "model": "qwen2.5-coder:7b",
      "system_prompt_file": "./prompts/privacy_filter.md",
      "timeout_seconds": 60
    }
  },
  "prefix_cache": {
    "enabled": true,
    "pin_system_first": true,
    "stable_tools": true,
    "skip_redaction_on_system": true
  },
  "compaction": {
    "enabled": true,
    "json_minify": true,
    "collapse_blank_lines": true,
    "trim_trailing_whitespace": true,
    "normalize_line_endings": true
  },
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
