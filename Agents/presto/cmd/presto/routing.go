package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultRouteConfigName = "model-routing.json"
	defaultDotEnvName      = ".env"
)

type routeConfig struct {
	DefaultRoute string                `json:"default_route"`
	Routes       map[string]modelRoute `json:"routes"`
}

type modelRoute struct {
	Provider   string `json:"provider,omitempty"`
	APIKeyEnv  string `json:"api_key_env,omitempty"`
	BaseURLEnv string `json:"base_url_env,omitempty"`
	ModelEnv   string `json:"model_env,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
}

func configureModelRoutingFromWorkspace() error {
	if envPath, ok := findUpward(defaultDotEnvName); ok {
		if err := applyDotEnv(envPath); err != nil {
			return err
		}
	}
	configPath := os.Getenv("MODEL_ROUTING_CONFIG")
	if configPath == "" {
		if found, ok := findUpward(defaultRouteConfigName); ok {
			configPath = found
		}
	}
	if configPath == "" {
		return nil
	}
	routeName, err := applyModelRoute(configPath)
	if err != nil {
		return err
	}
	if routeName != "" {
		os.Setenv("PRESTO_ACTIVE_ROUTE", routeName)
	}
	return nil
}

func applyDotEnv(path string) error {
	values, err := readDotEnv(path)
	if err != nil {
		return err
	}
	for key, value := range values {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyModelRoute(path string) (string, error) {
	config, err := readRouteConfig(path)
	if err != nil {
		return "", err
	}
	routeName := os.Getenv("PRESTO_ROUTE")
	if routeName == "" {
		routeName = config.DefaultRoute
	}
	if routeName == "" {
		return "", nil
	}
	route, ok := config.Routes[routeName]
	if !ok {
		return "", fmt.Errorf("model route %q not found in %s", routeName, path)
	}

	setIfEmptyFromValueOrEnv("PRESTO_API_KEY", route.APIKeyEnv, "")
	setIfEmptyFromValueOrEnv("PRESTO_BASE_URL", route.BaseURLEnv, route.BaseURL)
	setIfEmptyFromValueOrEnv("PRESTO_MODEL", route.ModelEnv, route.Model)
	return routeName, nil
}

func setIfEmptyFromValueOrEnv(target string, sourceEnv string, fallback string) {
	if os.Getenv(target) != "" {
		return
	}
	value := ""
	if sourceEnv != "" {
		value = os.Getenv(sourceEnv)
	}
	if value == "" {
		value = fallback
	}
	if value != "" {
		_ = os.Setenv(target, value)
	}
}

func readRouteConfig(path string) (routeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return routeConfig{}, err
	}
	var config routeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return routeConfig{}, err
	}
	if config.Routes == nil {
		config.Routes = map[string]modelRoute{}
	}
	return config, nil
}

func readDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid .env line in %s: %q", path, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			return nil, fmt.Errorf("empty .env key in %s", path)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func findUpward(name string) (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(cwd, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "", false
		}
		cwd = parent
	}
}

func requireRouteEnv() error {
	missing := make([]string, 0, 3)
	for _, key := range []string{"PRESTO_API_KEY", "PRESTO_BASE_URL", "PRESTO_MODEL"} {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return errors.New("missing routed Presto env: " + strings.Join(missing, ", "))
	}
	return nil
}
