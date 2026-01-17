package okrstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// PermissionConfig mirrors okrs/permissions.yml.
type PermissionConfig struct {
	Permissions struct {
		Read  []string `yaml:"read"`
		Write []string `yaml:"write"`
	} `yaml:"permissions"`

	// Delegations optionally maps owner_id -> list of agent_ids allowed to write.
	Delegations map[string][]string `yaml:"delegations"`
}

var (
	defaultPermissionsPath = filepath.Join("okrs", "permissions.yml")
	permOnce               sync.Once
	permCache              *PermissionConfig
	permErr                error
)

// LoadPermissionConfig reads the permissions YAML from the provided path.
func LoadPermissionConfig(path string) (*PermissionConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read permissions file: %w", err)
	}
	var cfg PermissionConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse permissions file: %w", err)
	}
	return &cfg, nil
}

func loadDefaultPermissions() (*PermissionConfig, error) {
	permOnce.Do(func() {
		cfg, err := LoadPermissionConfig(defaultPermissionsPath)
		if err != nil {
			permErr = err
			return
		}
		permCache = cfg
	})
	return permCache, permErr
}

func loadPermissionsForDir(dir string) (*PermissionConfig, error) {
	if dir == "" {
		return loadDefaultPermissions()
	}
	path := filepath.Join(dir, "permissions.yml")
	if _, err := os.Stat(path); err == nil {
		return LoadPermissionConfig(path)
	}
	return loadDefaultPermissions()
}

// CanPropose returns whether an agent may propose updates for the given owner_id.
func CanPropose(agentID, targetOwnerID string) bool {
	agentID = strings.TrimSpace(agentID)
	targetOwnerID = strings.TrimSpace(targetOwnerID)
	if agentID == "" || targetOwnerID == "" {
		return false
	}

	cfg, err := loadDefaultPermissions()
	if err != nil {
		return false
	}

	return canProposeWithConfig(cfg, agentID, targetOwnerID)
}

func canProposeWithConfig(cfg *PermissionConfig, agentID, targetOwnerID string) bool {
	if cfg == nil {
		return false
	}

	writeRules := make(map[string]struct{})
	for _, r := range cfg.Permissions.Write {
		writeRules[strings.TrimSpace(r)] = struct{}{}
	}

	if _, ok := writeRules["owner_id_match"]; ok && agentID == targetOwnerID {
		return true
	}

	if _, ok := writeRules["delegated_explicitly"]; ok {
		if cfg.isDelegated(agentID, targetOwnerID) {
			return true
		}
	}

	return false
}

func (c *PermissionConfig) isDelegated(agentID, ownerID string) bool {
	if c == nil || len(c.Delegations) == 0 {
		return false
	}
	agents := c.Delegations[ownerID]
	for _, candidate := range agents {
		if strings.TrimSpace(candidate) == agentID {
			return true
		}
	}
	return false
}
