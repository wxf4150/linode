package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	linodeAPIBase = "https://api.linode.com/v4"
	targetLabel   = "tmpnode"
)

// Config represents the configuration file structure
type Config struct {
	Token             string   `yaml:"token"`
	Image             string   `yaml:"image"`
	MaintenancePolicy string   `yaml:"maintenance_policy"`
	PrivateIP         bool     `yaml:"private_ip"`
	Region            string   `yaml:"region"`
	Type              string   `yaml:"type"`
	Label             string   `yaml:"label"`
	RootPass          string   `yaml:"root_pass"`
	AuthorizedUsers   []string `yaml:"authorized_users"`
	DiskEncryption    string   `yaml:"disk_encryption"`
}

// LinodeInstance represents a Linode instance
type LinodeInstance struct {
	ID     int    `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
}

// LinodeListResponse represents the response from list instances API
type LinodeListResponse struct {
	Data []LinodeInstance `json:"data"`
}

// LinodeCreateRequest represents the request to create an instance
type LinodeCreateRequest struct {
	Image             string   `json:"image"`
	MaintenancePolicy string   `json:"maintenance_policy,omitempty"`
	PrivateIP         bool     `json:"private_ip"`
	Region            string   `json:"region"`
	Type              string   `json:"type"`
	Label             string   `json:"label"`
	RootPass          string   `json:"root_pass"`
	AuthorizedUsers   []string `json:"authorized_users,omitempty"`
	DiskEncryption    string   `json:"disk_encryption,omitempty"`
}

func main() {
	// Load configuration
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Check command line arguments
	if len(os.Args) > 1 && os.Args[1] == "drop" {
		// Drop mode
		if err := dropMode(config); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Default mode (create/check)
		if err := defaultMode(config); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func loadConfig() (*Config, error) {
	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	// Try executable directory first
	configPath := filepath.Join(exeDir, "linode.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Fallback to current directory
		configPath = "linode.yaml"
		data, err = os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if config.Token == "" {
		return nil, fmt.Errorf("token is required in config file")
	}

	return &config, nil
}

func defaultMode(config *Config) error {
	fmt.Println("Checking for existing instances...")

	// List instances
	instances, err := listInstances(config.Token)
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	// Check if tmpnode exists
	var tmpNode *LinodeInstance
	for i := range instances {
		if instances[i].Label == targetLabel {
			tmpNode = &instances[i]
			break
		}
	}

	if tmpNode != nil {
		fmt.Printf("Instance with label '%s' already exists (ID: %d, Status: %s)\n", targetLabel, tmpNode.ID, tmpNode.Status)
		return nil
	}

	// Create new instance
	fmt.Printf("No instance with label '%s' found. Creating new instance...\n", targetLabel)

	instance, err := createInstance(config)
	if err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	fmt.Printf("Instance created with ID: %d\n", instance.ID)
	fmt.Println("Waiting for instance to be running...")

	// Poll for status
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		status, err := getInstanceStatus(config.Token, instance.ID)
		if err != nil {
			return fmt.Errorf("failed to get instance status: %w", err)
		}

		fmt.Printf("Instance status: %s\n", status)

		if status == "running" {
			fmt.Printf("Instance '%s' is now running!\n", targetLabel)
			break
		}
	}

	return nil
}

func dropMode(config *Config) error {
	fmt.Println("Checking for existing instances...")

	// List instances
	instances, err := listInstances(config.Token)
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	// Check if tmpnode exists
	var tmpNode *LinodeInstance
	for i := range instances {
		if instances[i].Label == targetLabel {
			tmpNode = &instances[i]
			break
		}
	}

	if tmpNode == nil {
		fmt.Printf("No instance with label '%s' found.\n", targetLabel)
		return nil
	}

	fmt.Printf("Found instance with label '%s' (ID: %d). Deleting...\n", targetLabel, tmpNode.ID)

	if err := deleteInstance(config.Token, tmpNode.ID); err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}

	fmt.Printf("Instance '%s' has been deleted successfully.\n", targetLabel)
	return nil
}

func listInstances(token string) ([]LinodeInstance, error) {
	req, err := http.NewRequest("GET", linodeAPIBase+"/linode/instances", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var listResp LinodeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, err
	}

	return listResp.Data, nil
}

func createInstance(config *Config) (*LinodeInstance, error) {
	createReq := LinodeCreateRequest{
		Image:             config.Image,
		MaintenancePolicy: config.MaintenancePolicy,
		PrivateIP:         config.PrivateIP,
		Region:            config.Region,
		Type:              config.Type,
		Label:             targetLabel, // Always use "tmpnode" as label
		RootPass:          config.RootPass,
		AuthorizedUsers:   config.AuthorizedUsers,
		DiskEncryption:    config.DiskEncryption,
	}

	jsonData, err := json.Marshal(createReq)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", linodeAPIBase+"/linode/instances", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+config.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var instance LinodeInstance
	if err := json.NewDecoder(resp.Body).Decode(&instance); err != nil {
		return nil, err
	}

	return &instance, nil
}

func getInstanceStatus(token string, instanceID int) (string, error) {
	url := fmt.Sprintf("%s/linode/instances/%d", linodeAPIBase, instanceID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var instance LinodeInstance
	if err := json.NewDecoder(resp.Body).Decode(&instance); err != nil {
		return "", err
	}

	return instance.Status, nil
}

func deleteInstance(token string, instanceID int) error {
	url := fmt.Sprintf("%s/linode/instances/%d", linodeAPIBase, instanceID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
