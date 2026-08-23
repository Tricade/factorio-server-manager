package factorio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialStatusLoadDoesNotDeleteInvalidState(t *testing.T) {
	originalPath := credentialsFilePath
	path := filepath.Join(t.TempDir(), "factorio.auth")
	credentialsFilePath = func() string { return path }
	t.Cleanup(func() { credentialsFilePath = originalPath })

	for name, contents := range map[string]string{
		"malformed":  `{not-json`,
		"incomplete": `{"username":"factorio-account"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			var credentials Credentials
			if authenticated, err := credentials.Load(); err == nil || authenticated {
				t.Fatalf("invalid credential state was accepted: authenticated=%t error=%v", authenticated, err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("status read deleted the credential file: %v", err)
			}
		})
	}
}
