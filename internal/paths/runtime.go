package paths

import (
	"os"
	"os/exec"
)

type Runtime string

const (
	Docker Runtime = "docker"
	Podman Runtime = "podman"
)

func DetectRuntime() Runtime {
	if r := os.Getenv("NENYACTL_RUNTIME"); r != "" {
		switch Runtime(r) {
		case Docker:
			return Docker
		case Podman:
			return Podman
		}
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return Podman
	}
	return Docker
}

func ComposeCmd() (string, []string, error) {
	switch r := DetectRuntime(); r {
	case Podman:
		return "podman", []string{"compose"}, nil
	default:
		return "docker", []string{"compose"}, nil
	}
}
