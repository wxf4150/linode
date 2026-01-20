# Linode Instance Manager

A simple command-line tool for managing Linode instances. This tool can automatically create and manage a temporary Linode instance with label "tmpnode".

## Features

- Automatically create a Linode instance if it doesn't exist
- Monitor instance creation progress
- Delete instances on demand
- Configuration via YAML file

## Prerequisites

- Go 1.16 or later
- A Linode account with a Personal Access Token
- API documentation: https://techdocs.akamai.com/linode-api/reference/get-linode-instances

## Getting Your API Token

1. Visit https://techdocs.akamai.com/linode-api/reference/get-started
2. Log in to your Linode account
3. Go to https://cloud.linode.com/profile/tokens
4. Create a new Personal Access Token with read/write permissions for Linodes
5. Copy the token (you won't be able to see it again!)

## Installation

### From Source

```bash
git clone https://github.com/wxf4150/linode.git
cd linode
go build -o linode-manager
```

## Configuration

Create a `linode.yaml` file in the same directory as the binary with the following content:

```yaml
# Your Linode Personal Access Token (required)
token: "YOUR_LINODE_API_TOKEN_HERE"

# Instance configuration parameters
image: "linode/ubuntu24.04"
maintenance_policy: "linode/migrate"
private_ip: false
region: "us-lax"
type: "g6-nanode-1"
label: "ubuntu-us-lax"
root_pass: "YOUR_SECURE_PASSWORD"
authorized_users:
  - "ubuntu"
disk_encryption: "disabled"
```

### Configuration Parameters

- `token`: Your Linode Personal Access Token (required)
- `image`: The Linode image to use (e.g., "linode/ubuntu24.04")
- `maintenance_policy`: How to handle maintenance (e.g., "linode/migrate")
- `private_ip`: Whether to assign a private IP (true/false)
- `region`: The data center region (e.g., "us-lax", "us-east", "eu-west")
- `type`: The Linode plan type (e.g., "g6-nanode-1")
- `label`: Default label for the instance
- `root_pass`: Root password for the instance
- `authorized_users`: List of authorized users
- `disk_encryption`: Disk encryption setting ("enabled" or "disabled")

## Usage

### Create/Check Instance (Default Mode)

Run the application without parameters to check for or create an instance with label "tmpnode":

```bash
./linode-manager
```

**Behavior:**
1. Lists all Linode instances
2. If no instance with label "tmpnode" exists:
   - Creates a new instance with label "tmpnode"
   - Monitors creation progress every 2 seconds
   - Prints status updates until the instance is running
3. If an instance with label "tmpnode" exists:
   - Prints a message and exits

**Example Output:**
```
Checking for existing instances...
No instance with label 'tmpnode' found. Creating new instance...
Instance created with ID: 12345678
Waiting for instance to be running...
Instance status: provisioning
Instance status: booting
Instance status: running
Instance 'tmpnode' is now running!
```

### Delete Instance (Drop Mode)

Run the application with the `drop` parameter to delete the "tmpnode" instance:

```bash
./linode-manager drop
```

**Behavior:**
1. Lists all Linode instances
2. If an instance with label "tmpnode" exists:
   - Deletes the instance
   - Prints confirmation message
3. If no instance with label "tmpnode" exists:
   - Prints a message and exits

**Example Output:**
```
Checking for existing instances...
Found instance with label 'tmpnode' (ID: 12345678). Deleting...
Instance 'tmpnode' has been deleted successfully.
```

## Common Issues

### "Error loading config: failed to read config file"

Make sure `linode.yaml` is in the same directory as the binary or in the current working directory.

### "API request failed with status 401"

Your API token is invalid or expired. Check your `linode.yaml` and ensure the token is correct.

### "API request failed with status 400"

One or more configuration parameters are invalid. Check the Linode API documentation for valid values for your region, type, and image.

## License

MIT License

## API Reference

This tool uses the Linode API v4. For more information, see:
- API Documentation: https://techdocs.akamai.com/linode-api/reference/get-linode-instances
- Getting Started: https://techdocs.akamai.com/linode-api/reference/get-started
