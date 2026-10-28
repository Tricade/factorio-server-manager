package factorio

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

var consoleKernel = windows.NewLazySystemDLL("kernel32.dll")
var sendWindowsControlEvent = sendCtrlCToPid
var setWindowsControlHandling = setCtrlHandlingIsDisabledForThisProcess

func callConsoleProcedure(name string, args ...uintptr) error {
	procedure := consoleKernel.NewProc(name)
	if err := procedure.Find(); err != nil {
		return fmt.Errorf("find console procedure %s: %w", name, err)
	}
	result, _, err := procedure.Call(args...)
	if result == 0 {
		return fmt.Errorf("call console procedure %s: %w", name, err)
	}
	return nil
}

func sendCtrlCToPid(pid int) error {
	return callConsoleProcedure("GenerateConsoleCtrlEvent", uintptr(syscall.CTRL_C_EVENT), uintptr(pid))
}

func setCtrlHandlingIsDisabledForThisProcess(disabled bool) error {
	var flag uintptr
	if disabled {
		flag = 1
	}
	return callConsoleProcedure("SetConsoleCtrlHandler", 0, flag)
}

func (server *Server) signalStop() (result error) {
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	// Console control is process-wide. Restore our handler even after a failed
	// signal, and return errors rather than terminating the entire manager.
	if err := setWindowsControlHandling(true); err != nil {
		server.SetStopping(false)
		return err
	}
	defer func() { result = errors.Join(result, setWindowsControlHandling(false)) }()
	if err := sendWindowsControlEvent(0); err != nil {
		server.SetStopping(false)
		return err
	}
	log.Print("Sent console shutdown signal to Factorio; waiting for clean shutdown")
	time.Sleep(20 * time.Millisecond)
	server.closeRcon()
	return nil
}

func (server *Server) checkProcessHealth(text string) {
	// Retain the original workaround for a closed session whose native process
	// has not exited. The common force-stop path handles process-exit races.
	if strings.Contains(text, "ServerMultiplayerManager.cpp:783: updateTick(0) changing state from(Disconnected) to(Closed)") {
		if err := server.Kill(); err != nil {
			log.Printf("Unable to finish Factorio shutdown: %v", err)
		}
	}
}
