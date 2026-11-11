//go:build windows

package win

// Windows service runtime and "launch a program into a user's session" helpers.
// These are what let Orbit Agent run as a SYSTEM service that supervises a UI
// agent in each signed-in user's session (so a student cannot simply close it).

import (
	"syscall"
	"unsafe"
)

var (
	advapi32 = sysDLL("advapi32.dll")
	wtsapi32 = sysDLL("wtsapi32.dll")
	userenv  = sysDLL("userenv.dll")

	pStartServiceCtrlDispatcherW   = advapi32.NewProc("StartServiceCtrlDispatcherW")
	pRegisterServiceCtrlHandlerExW = advapi32.NewProc("RegisterServiceCtrlHandlerExW")
	pSetServiceStatus              = advapi32.NewProc("SetServiceStatus")
	pDuplicateTokenEx              = advapi32.NewProc("DuplicateTokenEx")
	pCreateProcessAsUserW          = advapi32.NewProc("CreateProcessAsUserW")

	pWTSEnumerateSessionsW = wtsapi32.NewProc("WTSEnumerateSessionsW")
	pWTSFreeMemory         = wtsapi32.NewProc("WTSFreeMemory")
	pWTSQueryUserToken     = wtsapi32.NewProc("WTSQueryUserToken")

	pCreateEnvironmentBlock  = userenv.NewProc("CreateEnvironmentBlock")
	pDestroyEnvironmentBlock = userenv.NewProc("DestroyEnvironmentBlock")
)

// ---- service constants ----

const (
	SERVICE_WIN32_OWN_PROCESS = 0x00000010
	SERVICE_START_PENDING     = 0x00000002
	SERVICE_RUNNING           = 0x00000004
	SERVICE_STOP_PENDING      = 0x00000003
	SERVICE_STOPPED           = 0x00000001

	SERVICE_ACCEPT_STOP     = 0x00000001
	SERVICE_ACCEPT_SHUTDOWN = 0x00000004

	SERVICE_CONTROL_STOP        = 0x00000001
	SERVICE_CONTROL_SHUTDOWN    = 0x00000005
	SERVICE_CONTROL_INTERROGATE = 0x00000004

	errFailedServiceController = 1063 // ERROR_FAILED_SERVICE_CONTROLLER_CONNECT

	WTSActive = 0

	tokenPrimary          = 1
	securityImpersonation = 2
	createUnicodeEnv      = 0x00000400
	createNoWindow        = 0x08000000
	createNewConsole      = 0x00000010
	tokenAllAccess        = 0x000F01FF
	waitTimeout           = 0x00000102
)

// SERVICE_STATUS mirrors the Win32 structure.
type SERVICE_STATUS struct {
	ServiceType             uint32
	CurrentState            uint32
	ControlsAccepted        uint32
	Win32ExitCode           uint32
	ServiceSpecificExitCode uint32
	CheckPoint              uint32
	WaitHint                uint32
}

type serviceTableEntry struct {
	name *uint16
	proc uintptr
}

type startupInfoW struct {
	Cb            uint32
	Reserved      *uint16
	Desktop       *uint16
	Title         *uint16
	X             uint32
	Y             uint32
	XSize         uint32
	YSize         uint32
	XCountChars   uint32
	YCountChars   uint32
	FillAttribute uint32
	Flags         uint32
	ShowWindow    uint16
	CbReserved2   uint16
	Reserved2     *byte
	StdInput      uintptr
	StdOutput     uintptr
	StdError      uintptr
}

type processInformation struct {
	Process   uintptr
	Thread    uintptr
	ProcessID uint32
	ThreadID  uint32
}

type wtsSessionInfoW struct {
	SessionID      uint32
	WinStationName *uint16
	State          uint32
}

// ServiceStatusHandle is returned by RegisterServiceCtrlHandler.
type ServiceStatusHandle uintptr

// StartServiceDispatcher connects this process to the Service Control Manager
// and calls main as the service entry point. It blocks until the service stops.
// Returns false if the process was not started by the SCM (i.e. run from a
// console), so the caller can fall back to a normal run.
func StartServiceDispatcher(name string, main func()) bool {
	cb := syscall.NewCallback(func(argc uint32, argv **uint16) uintptr {
		main()
		return 0
	})
	table := []serviceTableEntry{{name: U16(name), proc: cb}, {}}
	r, _, _ := pStartServiceCtrlDispatcherW.Call(uintptr(unsafe.Pointer(&table[0])))
	return r != 0
}

// RegisterServiceHandler registers the control handler for the service.
// handler receives the control code (SERVICE_CONTROL_*).
func RegisterServiceHandler(name string, handler func(control uint32)) ServiceStatusHandle {
	cb := syscall.NewCallback(func(control, eventType, eventData, context uintptr) uintptr {
		handler(uint32(control))
		return 0
	})
	h, _, _ := pRegisterServiceCtrlHandlerExW.Call(uintptr(unsafe.Pointer(U16(name))), cb, 0)
	return ServiceStatusHandle(h)
}

// SetServiceState reports a new service state to the SCM.
func SetServiceState(h ServiceStatusHandle, state, accepted, waitHintMS uint32) {
	st := SERVICE_STATUS{
		ServiceType:      SERVICE_WIN32_OWN_PROCESS,
		CurrentState:     state,
		ControlsAccepted: accepted,
		WaitHint:         waitHintMS,
	}
	pSetServiceStatus.Call(uintptr(h), ptr(&st))
}

// SessionUser describes one interactive Windows session.
type SessionUser struct {
	SessionID uint32
	Active    bool
}

// ActiveSessions lists the terminal-server sessions and whether each is active
// (a user is signed in and using it).
func ActiveSessions() []SessionUser {
	var p uintptr
	var count uint32
	r, _, _ := pWTSEnumerateSessionsW.Call(0, 0, 1, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&count)))
	if r == 0 || p == 0 {
		return nil
	}
	defer pWTSFreeMemory.Call(p)
	size := unsafe.Sizeof(wtsSessionInfoW{})
	out := make([]SessionUser, 0, count)
	for i := uint32(0); i < count; i++ {
		s := (*wtsSessionInfoW)(unsafe.Pointer(p + uintptr(i)*size))
		out = append(out, SessionUser{SessionID: s.SessionID, Active: s.State == WTSActive})
	}
	return out
}

// LaunchInSession starts exe (with command line args) inside the given session,
// running as the session's signed-in user, on the interactive desktop. Returns
// a process handle (for liveness checks with ProcessAlive) and the PID.
func LaunchInSession(sessionID uint32, exe, args, workdir string) (handle uintptr, pid uint32, err error) {
	var userTok uintptr
	r, _, e := pWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&userTok)))
	if r == 0 {
		return 0, 0, e
	}
	defer syscall.CloseHandle(syscall.Handle(userTok))

	var primary uintptr
	r, _, e = pDuplicateTokenEx.Call(userTok, tokenAllAccess, 0, securityImpersonation, tokenPrimary, uintptr(unsafe.Pointer(&primary)))
	if r == 0 {
		return 0, 0, e
	}
	defer syscall.CloseHandle(syscall.Handle(primary))

	var env uintptr
	pCreateEnvironmentBlock.Call(uintptr(unsafe.Pointer(&env)), primary, 0)
	if env != 0 {
		defer pDestroyEnvironmentBlock.Call(env)
	}

	cmd := `"` + exe + `"`
	if args != "" {
		cmd += " " + args
	}
	cmdLine := U16(cmd)
	si := startupInfoW{Desktop: U16(`winsta0\default`)}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi processInformation
	flags := uintptr(createUnicodeEnv | createNoWindow)
	var wd uintptr
	if workdir != "" {
		wd = uintptr(unsafe.Pointer(U16(workdir)))
	}
	r, _, e = pCreateProcessAsUserW.Call(primary, 0, uintptr(unsafe.Pointer(cmdLine)),
		0, 0, 0, flags, env, wd, uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi)))
	if r == 0 {
		return 0, 0, e
	}
	syscall.CloseHandle(syscall.Handle(pi.Thread))
	return pi.Process, pi.ProcessID, nil
}

// ProcessAlive reports whether a process handle is still running.
func ProcessAlive(handle uintptr) bool {
	if handle == 0 {
		return false
	}
	ev, err := syscall.WaitForSingleObject(syscall.Handle(handle), 0)
	return err == nil && ev == waitTimeout
}

// CloseHandleSafe closes a handle, ignoring errors.
func CloseHandleSafe(handle uintptr) {
	if handle != 0 {
		syscall.CloseHandle(syscall.Handle(handle))
	}
}

// IsServiceControllerConnect reports whether err is the "not started by SCM" error.
func IsServiceControllerConnect(err error) bool {
	return err == syscall.Errno(errFailedServiceController)
}
