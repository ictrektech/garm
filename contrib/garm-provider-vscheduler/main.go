// Command garm-provider-vscheduler is a GARM external provider. It runs
// GitHub/Gitea Actions runners as V-Scheduler backend deployments, so one GARM
// controller can place runners on every node registered in V-Scheduler.
package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
)

const (
	// exitCodeNotFound is the exit code GARM expects for "instance does not exist".
	exitCodeNotFound = 30
	// removeTimeout bounds how long DeleteInstance waits for V-Scheduler to
	// confirm container removal. V-Scheduler retries removal every 4 seconds.
	removeTimeoutSeconds = 40
)

type notFoundError struct{ msg string }

func (e notFoundError) Error() string { return e.msg }

func main() {
	if err := run(); err != nil {
		var nf notFoundError
		if errors.As(err, &nf) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(exitCodeNotFound)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flag.Parse()

	if command := os.Getenv("GARM_COMMAND"); command == "GetVersion" {
		fmt.Println("v0.1.1")
		return nil
	}

	cfg, err := loadConfig(os.Getenv("GARM_PROVIDER_CONFIG_FILE"))
	if err != nil {
		return err
	}

	api := newAPI(cfg)
	poolID := os.Getenv("GARM_POOL_ID")
	instanceID := os.Getenv("GARM_INSTANCE_ID")

	switch os.Getenv("GARM_COMMAND") {
	case "CreateInstance":
		return createInstance(api, cfg, poolID)
	case "GetInstance":
		return getInstance(api, instanceID)
	case "ListInstances":
		return listInstances(api, poolID)
	case "DeleteInstance":
		return deleteInstance(api, instanceID)
	case "RemoveAllInstances":
		return removeAllInstances(api, poolID)
	case "StartInstance":
		return startInstance(api, instanceID)
	case "StopInstance":
		return stopInstance(api, instanceID)
	default:
		return fmt.Errorf("unsupported GARM_COMMAND %q", os.Getenv("GARM_COMMAND"))
	}
}

// poolSpecsFromEnv decodes GARM_POOL_EXTRASPECS, the pool level settings.
func poolSpecsFromEnv() (poolSpecs, error) {
	encoded := os.Getenv("GARM_POOL_EXTRASPECS")
	if encoded == "" {
		return poolSpecs{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return poolSpecs{}, fmt.Errorf("decoding GARM_POOL_EXTRASPECS: %w", err)
	}
	var specs poolSpecs
	if err := json.Unmarshal(raw, &specs); err != nil {
		return poolSpecs{}, fmt.Errorf("parsing GARM_POOL_EXTRASPECS: %w", err)
	}
	return specs, nil
}
