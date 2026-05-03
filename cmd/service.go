package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the Nenya service",
	Long:  `Start, stop, reload, or check the status of the Nenya service.`,
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Nenya service",
	RunE:  runServiceStart,
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Nenya service",
	RunE:  runServiceStop,
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the Nenya service status",
	RunE:  runServiceStatus,
}

var serviceReloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload the Nenya service configuration (SIGHUP)",
	RunE:  runServiceReload,
}

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
	serviceCmd.AddCommand(serviceReloadCmd)
}

func runServiceStart(cmd *cobra.Command, args []string) error {
	switch runtime.GOOS {
	case "linux":
		return systemctl("start", "nenya")
	case "darwin":
		return launchctl("load", "/Library/LaunchDaemons/com.gumieri.nenya.plist")
	default:
		return fmt.Errorf("service management not supported on %s", runtime.GOOS)
	}
}

func runServiceStop(cmd *cobra.Command, args []string) error {
	switch runtime.GOOS {
	case "linux":
		return systemctl("stop", "nenya")
	case "darwin":
		return launchctl("unload", "/Library/LaunchDaemons/com.gumieri.nenya.plist")
	default:
		return fmt.Errorf("service management not supported on %s", runtime.GOOS)
	}
}

func runServiceStatus(cmd *cobra.Command, args []string) error {
	switch runtime.GOOS {
	case "linux":
		return systemctl("status", "nenya")
	case "darwin":
		return showLaunchdStatus()
	default:
		return fmt.Errorf("service management not supported on %s", runtime.GOOS)
	}
}

func showLaunchdStatus() error {
	// launchctl list | grep com.gumieri.nenya
	grep := exec.Command("grep", "com.gumieri.nenya")
	list := exec.Command("launchctl", "list")
	pipe, err := list.StdoutPipe()
	if err != nil {
		return err
	}
	grep.Stdin = pipe
	grep.Stdout = nil

	if err := list.Start(); err != nil {
		return fmt.Errorf("launchctl list: %w", err)
	}
	out, err := grep.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nenya service not found in launchd")
	}
	fmt.Print(string(out))
	return nil
}

func runServiceReload(cmd *cobra.Command, args []string) error {
	switch runtime.GOOS {
	case "linux":
		return systemctl("reload", "nenya")
	case "darwin":
		if err := launchctl("unload", "/Library/LaunchDaemons/com.gumieri.nenya.plist"); err != nil {
			return err
		}
		return launchctl("load", "/Library/LaunchDaemons/com.gumieri.nenya.plist")
	default:
		return fmt.Errorf("service management not supported on %s", runtime.GOOS)
	}
}

func systemctl(action, unit string) error {
	cmd := exec.Command("systemctl", action, unit)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s %s: %w\n%s", action, unit, err, string(out))
	}
	fmt.Print(string(out))
	return nil
}

func launchctl(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %v: %w\n%s", args, err, string(out))
	}
	fmt.Print(string(out))
	return nil
}
