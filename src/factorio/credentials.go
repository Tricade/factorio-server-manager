package factorio

import (
	"encoding/json"
	"errors"
	"log"
	"os"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

type Credentials struct {
	Username string `json:"username"`
	Userkey  string `json:"userkey"`
}

var credentialsFilePath = func() string {
	return bootstrap.GetConfig().FactorioCredentialsFile
}

func (credentials *Credentials) Save() error {
	var err error
	credentialsJson, err := json.Marshal(credentials)
	if err != nil {
		log.Printf("error mashalling the credentials: %s", err)
		return err
	}

	path := credentialsFilePath()
	err = os.WriteFile(path, credentialsJson, 0600)
	if err != nil {
		log.Printf("error on saving the credentials. %s", err)
		return err
	}
	if err = os.Chmod(path, 0600); err != nil {
		log.Printf("error restricting credential file permissions: %s", err)
		return err
	}

	return nil
}

func (credentials *Credentials) Load() (bool, error) {
	var err error
	path := credentialsFilePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}

	fileBytes, err := os.ReadFile(path)
	if err != nil {
		log.Printf("error reading CredentialsFile: %s", err)
		return false, err
	}

	err = json.Unmarshal(fileBytes, credentials)
	if err != nil {
		log.Printf("error on unmarshal credentials_file: %s", err)
		return false, err
	}

	if credentials.Userkey != "" && credentials.Username != "" {
		return true, nil
	} else {
		return false, errors.New("incredients incomplete")
	}
}

func (credentials *Credentials) Del() error {
	var err error
	err = os.Remove(credentialsFilePath())
	if err != nil {
		log.Printf("error delete the credentialfile: %s", err)
		return err
	}

	return nil
}
