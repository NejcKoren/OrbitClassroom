//go:build !windows

package agent

import "errors"

// Stubs so the package builds on non-Windows. The service only exists on Windows.

const ServiceName = "OrbitAgentSvc"

// RunService is a no-op on non-Windows.
func RunService() int { return 3 }

// InstallService is unsupported off Windows.
func InstallService(exe string) error { return errors.New("service mode is only supported on Windows") }

// RemoveService is a no-op on non-Windows.
func RemoveService() {}
