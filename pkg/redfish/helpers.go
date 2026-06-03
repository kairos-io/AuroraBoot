package redfish

import "github.com/stmcginnis/gofish/schemas"

// mediaSupportsCD reports whether a VirtualMedia resource advertises CD or DVD
// media support.
func mediaSupportsCD(vm *schemas.VirtualMedia) bool {
	for _, t := range vm.MediaTypes {
		if t == schemas.CDVirtualMediaType || t == schemas.DVDVirtualMediaType {
			return true
		}
	}
	return false
}

// bootSource maps our BootTarget to the gofish BootSource value, defaulting to Cd.
func bootSource(t BootTarget) schemas.BootSource {
	switch t {
	case BootTargetUSB:
		return schemas.UsbBootSource
	case BootTargetCd:
		return schemas.CdBootSource
	default:
		return schemas.CdBootSource
	}
}

// bootMode maps our BootMode to the gofish BootSourceOverrideMode, defaulting to UEFI.
func bootMode(m BootMode) schemas.BootSourceOverrideMode {
	switch m {
	case BootModeLegacy:
		return schemas.LegacyBootSourceOverrideMode
	case BootModeUEFI:
		return schemas.UEFIBootSourceOverrideMode
	default:
		return schemas.UEFIBootSourceOverrideMode
	}
}

// chooseResetType picks a sensible ResetType from the system's current power
// state, validated against the advertised allowable values when present. Off
// systems are powered On; running systems get a ForceRestart (falling back to
// GracefulRestart, then On) to drive the install boot.
func chooseResetType(system *schemas.ComputerSystem) schemas.ResetType {
	allowed, _ := system.GetSupportedResetTypes()

	if system.PowerState == schemas.OffPowerState {
		return firstSupported(allowed, schemas.OnResetType, schemas.ForceOnResetType)
	}
	return firstSupported(allowed,
		schemas.ForceRestartResetType,
		schemas.GracefulRestartResetType,
		schemas.OnResetType,
	)
}

// firstSupported returns the first preference present in allowed; if allowed is
// empty (the BMC advertised nothing) it returns the first preference unchanged.
func firstSupported(allowed []schemas.ResetType, preferences ...schemas.ResetType) schemas.ResetType {
	if len(allowed) == 0 {
		return preferences[0]
	}
	for _, pref := range preferences {
		for _, a := range allowed {
			if a == pref {
				return pref
			}
		}
	}
	// None of our preferences are allowed; fall back to the first preference and
	// let the BMC reject it with a useful error.
	return preferences[0]
}

// isTerminalTaskState reports whether a Redfish TaskState is final.
func isTerminalTaskState(s schemas.TaskState) bool {
	switch s {
	case schemas.CompletedTaskState,
		schemas.ExceptionTaskState,
		schemas.KilledTaskState,
		schemas.CancelledTaskState:
		return true
	default:
		return false
	}
}

// taskMessages flattens a Task's messages into human-readable strings.
func taskMessages(task *schemas.Task) []string {
	if len(task.Messages) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(task.Messages))
	for _, m := range task.Messages {
		if m.Message != "" {
			msgs = append(msgs, m.Message)
		}
	}
	return msgs
}

// pickTaskURI returns the first non-empty Task monitor URI among the supplied
// TaskMonitorInfos, in preference order. A nil or empty TaskMonitor means the BMC
// answered synchronously and there is nothing to poll.
func pickTaskURI(infos ...*schemas.TaskMonitorInfo) string {
	for _, info := range infos {
		if info == nil {
			continue
		}
		if info.TaskMonitor != "" {
			return info.TaskMonitor
		}
		if info.Task != nil && info.Task.ODataID != "" {
			return info.Task.ODataID
		}
	}
	return ""
}
