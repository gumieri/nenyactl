package cmd

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunVersion(t *testing.T) {
	t.Run("errors gracefully", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		cmd := &cobra.Command{Use: "version"}
		cmd.SetContext(ctx)
		if err := runVersion(cmd, nil); err != nil {
			t.Fatalf("runVersion() error = %v", err)
		}
	})
}
