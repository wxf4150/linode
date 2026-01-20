package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	linodeAPIBase = "https://api.linode.com/v4"
	targetLabel   = "tmpnode"
	clearPadding  = "     " // Padding to clear previous output on the same line
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
	ID     int      `json:"id"`
	Label  string   `json:"label"`
	Status string   `json:"status"`
	Ipv4   []string `json:"ipv4"`
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
		fmt.Printf("Instance with label '%s' already exists (ID: %d, Status: %s, Ip: %s)\n", targetLabel, tmpNode.ID, tmpNode.Status, tmpNode.Ipv4)
		return nil
	}

	// Create new instance
	fmt.Printf("No instance with label '%s' found. Creating new instance...\n", targetLabel)

	instance, err := createInstance(config)
	if err != nil {
		return fmt.Errorf("failed to create instance: %w", err)
	}

	fmt.Printf("Instance created with ID: %d\n", instance.ID)

	// Track start time
	startTime := time.Now()

	// Poll for status with inline updates
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	dots := []string{"", ".", "..", "..."}
	dotIndex := 0

	for {
		<-ticker.C

		linodeInstance, err := getInstanceStatus(config.Token, instance.ID)
		if err != nil {
			return fmt.Errorf("failed to get instance status: %w", err)
		}

		// Print linodeInstance inline with animated dots
		fmt.Printf("\rInstance status: %s%s%s", linodeInstance.Status, dots[dotIndex], clearPadding)
		dotIndex = (dotIndex + 1) % len(dots)

		if linodeInstance.Status == "running" {
			elapsed := time.Since(startTime)
			fmt.Printf("\rInstance '%s' is now running! (Time taken: %.1f seconds)\n", targetLabel, elapsed.Seconds())
			if len(linodeInstance.Ipv4) > 0 {
				ipAddress := linodeInstance.Ipv4[0]
				fmt.Printf("Instance IP Address: %s\n", ipAddress)

				// Update hosts file
				fmt.Printf("Updating hosts file with entry: %s\t%s\n", ipAddress, targetLabel)
				if err := updateHostsFile(ipAddress, targetLabel); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to update hosts file: %v\n", err)
					fmt.Fprintf(os.Stderr, "You can manually add the following entry to your hosts file:\n")
					fmt.Fprintf(os.Stderr, "%s\t%s\n", ipAddress, targetLabel)
				} else {
					fmt.Printf("Successfully added hosts file entry: %s\t%s\n", ipAddress, targetLabel)
				}
			} else {
				fmt.Println("No IPv4 address found for the instance.")
			}
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

	// Remove hosts file entry
	fmt.Printf("Removing hosts file entry for '%s'\n", targetLabel)
	if err := removeHostsFileEntry(targetLabel); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to remove hosts file entry: %v\n", err)
	} else {
		fmt.Printf("Successfully removed hosts file entry for '%s'\n", targetLabel)
	}

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

func getInstanceStatus(token string, instanceID int) (*LinodeInstance, error) {
	url := fmt.Sprintf("%s/linode/instances/%d", linodeAPIBase, instanceID)

	req, err := http.NewRequest("GET", url, nil)
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

	var instance LinodeInstance
	if err := json.NewDecoder(resp.Body).Decode(&instance); err != nil {
		return nil, err
	}

	return &instance, nil
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

// getHostsFilePath returns the hosts file path based on the operating system
func getHostsFilePath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("SystemRoot"), "System32", "drivers", "etc", "hosts"), nil
	case "darwin", "linux":
		return "/etc/hosts", nil
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// updateHostsFile adds or updates a hosts file entry for the given IP and hostname
func updateHostsFile(ip, hostname string) error {
	hostsPath, err := getHostsFilePath()
	if err != nil {
		return err
	}

	// Read the current hosts file
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w (you may need to run with administrator/sudo privileges)", err)
	}

	// Parse existing hosts file and check if entry already exists
	lines := strings.Split(string(data), "\n")
	var newLines []string
	entryExists := false
	entryPattern := fmt.Sprintf("%s\t%s", ip, hostname)
	commentMarker := fmt.Sprintf("# Added by linode-manager for %s", hostname)

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		// Skip the old comment and entry for this specific hostname
		if trimmedLine == commentMarker {
			continue
		}
		// Check if this line has an entry for our exact hostname (not a substring match)
		if !strings.HasPrefix(trimmedLine, "#") && trimmedLine != "" {
			fields := strings.Fields(trimmedLine)
			if len(fields) >= 2 && fields[1] == hostname {
				// Replace with new IP
				newLines = append(newLines, commentMarker)
				newLines = append(newLines, entryPattern)
				entryExists = true
				continue
			}
		}
		newLines = append(newLines, line)
	}

	// If entry doesn't exist, add it
	if !entryExists {
		// Ensure the file ends with a newline before adding new entry
		if len(newLines) > 0 && newLines[len(newLines)-1] != "" {
			newLines = append(newLines, "")
		}
		newLines = append(newLines, commentMarker)
		newLines = append(newLines, entryPattern)
	}

	// Write back to hosts file
	newData := strings.Join(newLines, "\n")
	if err := os.WriteFile(hostsPath, []byte(newData), 0644); err != nil {
		return fmt.Errorf("failed to write hosts file: %w (you may need to run with administrator/sudo privileges)", err)
	}

	return nil
}

// removeHostsFileEntry removes a hosts file entry for the given hostname
func removeHostsFileEntry(hostname string) error {
	hostsPath, err := getHostsFilePath()
	if err != nil {
		return err
	}

	// Read the current hosts file
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %w", err)
	}

	// Parse existing hosts file and remove entry
	lines := strings.Split(string(data), "\n")
	var newLines []string
	commentMarker := fmt.Sprintf("# Added by linode-manager for %s", hostname)
	skipNext := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Skip the comment line for this hostname
		if trimmedLine == commentMarker {
			skipNext = true
			continue
		}

		// Skip the entry line that follows the comment
		if skipNext {
			if !strings.HasPrefix(trimmedLine, "#") && trimmedLine != "" {
				fields := strings.Fields(trimmedLine)
				if len(fields) >= 2 && fields[1] == hostname {
					skipNext = false
					continue
				}
			}
			// If it's not our entry, keep the line
			skipNext = false
		}

		newLines = append(newLines, line)
	}

	// Write back to hosts file
	newData := strings.Join(newLines, "\n")
	if err := os.WriteFile(hostsPath, []byte(newData), 0644); err != nil {
		return fmt.Errorf("failed to write hosts file: %w", err)
	}

	return nil
}
