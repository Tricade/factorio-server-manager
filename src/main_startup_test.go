package main

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestStartupFailureExitsNonzeroInSubprocess(t *testing.T) {
	if os.Getenv("FSM_STARTUP_FAILURE_HELPER") == "1" {
		failedStep := os.Getenv("FSM_STARTUP_FAILURE_STEP")
		step := func(name string) func() error {
			return func() error {
				if failedStep == name {
					return errors.New("injected startup failure")
				}
				return nil
			}
		}
		os.Exit(initializeManager(step("preparation"), step("engine"), step("profiles")))
	}

	for _, failedStep := range []string{"preparation", "engine", "profiles"} {
		t.Run(failedStep, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestStartupFailureExitsNonzeroInSubprocess$")
			cmd.Env = append(os.Environ(), "FSM_STARTUP_FAILURE_HELPER=1", "FSM_STARTUP_FAILURE_STEP="+failedStep)
			if err := cmd.Run(); err == nil {
				t.Fatal("startup subprocess unexpectedly succeeded")
			} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
				t.Fatalf("startup subprocess exit = %v; want exit code 1", err)
			}
		})
	}
}
