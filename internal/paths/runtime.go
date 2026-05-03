package paths

import (
	"os/exec"
)

type Runtime string

const (
	Docker Runtime = "docker"
	Podman Runtime = "podman"
)

func DetectRuntime() Runtime {
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
