package cmd

import (
	"fmt"
	"io"
	"runtime"
	"strings"

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
	return runServiceStartWithExec(defaultExec)
}

func runServiceStartWithExec(ex execer) error {
	switch runtime.GOOS {
	case "linux":
		return systemctlWithExec(ex, "start", "nenya")
	case "darwin":
		return launchctlWithExec(ex, "load", "/Library/LaunchDaemons/com.gumieri.nenya.plist")
	default:
		return fmt.Errorf("service management not supported on %s; use 'nenyactl containers setup' instead", runtime.GOOS)
	}
}

func runServiceStop(cmd *cobra.Command, args []string) error {
	return runServiceStopWithExec(defaultExec)
}

func runServiceStopWithExec(ex execer) error {
	switch runtime.GOOS {
	case "linux":
		return systemctlWithExec(ex, "stop", "nenya")
	case "darwin":
		return launchctlWithExec(ex, "unload", "/Library/LaunchDaemons/com.gumieri.nenya.plist")
	default:
		return fmt.Errorf("service management not supported on %s; use 'nenyactl containers setup' instead", runtime.GOOS)
	}
}

func runServiceStatus(cmd *cobra.Command, args []string) error {
	return runServiceStatusWithExec(defaultExec)
}

func runServiceStatusWithExec(ex execer) error {
	switch runtime.GOOS {
	case "linux":
		return systemctlWithExec(ex, "status", "nenya")
	case "darwin":
		return showLaunchdStatusWithExec(ex)
	default:
		return fmt.Errorf("service management not supported on %s; use 'nenyactl containers setup' instead", runtime.GOOS)
	}
}

func showLaunchdStatusWithExec(ex execer) error {
	var buf strings.Builder
	c := ex.Command("launchctl", "list").Stdout(&buf).Stderr(io.Discard)
	if err := c.Run(); err != nil {
		return fmt.Errorf("launchctl list: %w", err)
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "com.gumieri.nenya") {
			fmt.Println(line)
			return nil
		}
	}
	return fmt.Errorf("nenya service not found in launchd")
}

func runServiceReload(cmd *cobra.Command, args []string) error {
	return runServiceReloadWithExec(defaultExec)
}

func runServiceReloadWithExec(ex execer) error {
	switch runtime.GOOS {
	case "linux":
		return systemctlWithExec(ex, "reload", "nenya")
	case "darwin":
		if err := launchctlWithExec(ex, "unload", "/Library/LaunchDaemons/com.gumieri.nenya.plist"); err != nil {
			return err
		}
		return launchctlWithExec(ex, "load", "/Library/LaunchDaemons/com.gumieri.nenya.plist")
	default:
		return fmt.Errorf("service management not supported on %s; use 'nenyactl containers setup' instead", runtime.GOOS)
	}
}

func systemctlWithExec(ex execer, action, unit string) error {
	var buf strings.Builder
	c := ex.Command("systemctl", action, unit).Stdout(&buf).Stderr(io.Discard)
	if err := c.Run(); err != nil {
		return fmt.Errorf("systemctl %s %s: %w\n%s", action, unit, err, buf.String())
	}
	fmt.Print(buf.String())
	return nil
}

func systemctl(action, unit string) error {
	return systemctlWithExec(defaultExec, action, unit)
}

func launchctlWithExec(ex execer, args ...string) error {
	var buf strings.Builder
	c := ex.Command("launchctl", args...).Stdout(&buf).Stderr(io.Discard)
	if err := c.Run(); err != nil {
		return fmt.Errorf("launchctl %v: %w\n%s", args, err, buf.String())
	}
	fmt.Print(buf.String())
	return nil
}

func launchctl(args ...string) error {
	return launchctlWithExec(defaultExec, args...)
}
