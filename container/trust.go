package container

import (
	"os"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/reference"
	"github.com/docker/docker/cli/command"
	"github.com/docker/docker/cliconfig"
	"github.com/docker/docker/cliconfig/configfile"
	"github.com/docker/docker/cliconfig/credentials"
)

/**
 * Return an encoded auth config for the given registry
 * loaded from environment variables or docker config
 * as available in that order
 */
func EncodedAuth(ref string) (string, error) {
	auth, err := firstValidAuth(ref, []authBackend{
		authFromEnv(),
		authFromDockerConfig(),
	})
	if err != nil {
		log.Debugf("Loaded auth credentials %s for %s", auth, ref)
		return EncodeAuth(auth)
	}
	return "", err
}

// authBackend encapsulates a function that resolves registry credentials.
type authBackend func(string) (*types.AuthConfig, error)

// firstValidAuth tries a list of auth backends, returning first error or AuthConfig
func firstValidAuth(repo string, backends []authBackend) (*types.AuthConfig, error) {
	for _, backend := range backends {
		auth, err := backend(repo)
		if auth != nil || err != nil {
			return auth, err
		}
	}
	return nil, nil
}

// authFromEnv generates an authBackend via ENV variables
func authFromEnv() authBackend {
	return func(string) (*types.AuthConfig, error) {
		username := os.Getenv("REPO_USER")
		password := os.Getenv("REPO_PASS")
		if username != "" && password != "" {
			auth := types.AuthConfig{
				Username: username,
				Password: password,
			}
			return &auth, nil
		} else {
			return nil, nil
		}
	}
}

// authFromDockerConfig parses a Docker configuration for auth information
func authFromDockerConfig() authBackend {
	return func(ref string) (*types.AuthConfig, error) {
		server, err := ParseServerAddress(ref)
		configDir := os.Getenv("DOCKER_CONFIG")
		if configDir == "" {
			configDir = "/"
		}
		configFile, err := cliconfig.Load(configDir)
		if err != nil {
			log.Errorf("Unable to find default config file %s", err)
			return nil, err
		}

		credStore := CredentialsStore(*configFile, server)
		auth, err := credStore.Get(server) // returns (types.AuthConfig{}) if server not in credStore
		if auth == (types.AuthConfig{}) {
			return nil, nil
		}
		return &auth, nil
	}
}


func ParseServerAddress(ref string) (string, error) {
	repository, _, err := reference.Parse(ref)
	if err != nil {
		return ref, err
	}
	parts := strings.Split(repository, "/")
	return parts[0], nil
}

// CredentialsStore returns a new credentials store based
// on the settings provided in the configuration file.
func CredentialsStore(configFile configfile.ConfigFile, server string) credentials.Store {
	if configFile.CredentialsStore != "" {
		return credentials.NewNativeStore(&configFile, configFile.CredentialsStore)
	}
	helper, ok := configFile.CredentialHelpers[server]
	if ok {
		return credentials.NewNativeStore(&configFile, helper)
	}
	return credentials.NewFileStore(&configFile)
}

/*
 * Base64 encode an AuthConfig struct for transmission over HTTP
 */
func EncodeAuth(auth *types.AuthConfig) (string, error) {
	return command.EncodeAuthToBase64(*auth)
}

/**
 * This function will be invoked if an AuthConfig is rejected
 * It could be used to return a new value for the "X-Registry-Auth" authentication header,
 * but there's no point trying again with the same value as used in AuthConfig
 */
func DefaultAuthHandler() (string, error) {
	log.Debug("Authentication request was rejected. Trying again without authentication")
	return "", nil
}
