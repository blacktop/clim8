# Eight Sleep Daemon Scheduler

The `clim8 daemon` command runs a background scheduler that automatically controls your Eight Sleep Pod based on a configured schedule.

## Configuration

Create a config file at `~/.config/clim8/config.yaml` with your schedule:

```yaml
# Your Eight Sleep credentials
email: "your-email@example.com"
password: "your-password"

# Schedule configuration
schedule:
  # Turn on the pod at 10:00 PM
  - time: "22:00"
    action: "on"
  
  # Set temperature to 68°F at 10:15 PM
  - time: "22:15"
    action: "temp"
    temperature: "68F"
  
  # Lower temperature for deeper sleep at 1:00 AM
  - time: "01:00"
    action: "temp"
    temperature: "65F"
  
  # Slightly warm up before wake time at 5:30 AM
  - time: "05:30"
    action: "temp"
    temperature: "70F"
  
  # Turn off the pod at 6:00 AM
  - time: "06:00"
    action: "off"

# Optional: Enable verbose logging
verbose: false
```

## Available Actions

- **`on`** - Turn on the Eight Sleep pod
- **`off`** - Turn off the Eight Sleep pod  
- **`temp`** - Set temperature (requires `temperature` field with F or C suffix)

## Sides

Every item controls your own side of the bed unless it names another one:

```yaml
schedule:
  - time: "22:00"
    action: "on"
    side: "both"       # "left", "right" or "both"
  - time: "22:15"
    action: "temp"
    temperature: "66F"
    side: "right"
```

State sync (`--sync-state`) tracks each `side` value separately, so items for one side never
override the expected state of another. Avoid mixing `both` with `left`/`right` items for the
same period: they are tracked as independent schedules and would correct each other.

## Time Format

Times must be in 24-hour format (HH:MM):
- `"09:00"` - 9:00 AM
- `"22:30"` - 10:30 PM
- `"01:15"` - 1:15 AM

## Temperature Format

Temperatures must include the unit suffix:
- `"68F"` - 68 degrees Fahrenheit
- `"24C"` - 24 degrees Celsius
- `"72F"` - 72 degrees Fahrenheit

## Usage

### Run the daemon
```bash
clim8 daemon
```

### Test your schedule (dry run)
```bash
clim8 daemon --dry-run
```

### Run with verbose logging
```bash
clim8 daemon --verbose
```

### Run with custom timezone
```bash
clim8 daemon --timezone "America/Los_Angeles"
```

### Disable state synchronization (not recommended)
```bash
clim8 daemon --sync-state=false
```

### Run in background (macOS/Linux)
```bash
nohup clim8 daemon > ~/clim8.log 2>&1 &
```

## Features

- **Duplicate Prevention**: Each scheduled action only runs once per day
- **Graceful Shutdown**: Responds to SIGINT/SIGTERM signals
- **Error Recovery**: Continues running even if individual actions fail
- **Dry Run Mode**: Test your schedule without executing actions
- **Detailed Logging**: Shows what actions are being executed and when
- **Single Instance**: Prevents multiple daemons from running simultaneously
- **Security Checks**: Warns if config file has insecure permissions
- **State Synchronization**: Automatically checks and corrects device state after system wake/hibernation

## Security Considerations

### Config File Permissions
The daemon checks that your config file has secure permissions and warns if it's readable by others:

```bash
# Secure your config file
chmod 600 ~/.config/clim8/config.yaml
```

### Credential Storage
Your Eight Sleep credentials are stored in plaintext in the config file. Consider:
- Setting restrictive file permissions (600)
- Using environment variables instead:
  ```bash
  export CLIM8_EMAIL="your-email@example.com"
  export CLIM8_PASSWORD="your-password"
  ```

  > [!NOTE]
  > Environment variables won't work when ran as a `brew service`

### Process Management
The daemon creates a PID file to prevent multiple instances:
- PID file location: `~/.config/clim8/daemon.pid`
- Automatically cleaned up on shutdown

## Example Sleep Schedule

Here's a complete example for optimized sleep:

```yaml
email: "your-email@example.com"
password: "your-password"

schedule:
  # Pre-bedtime routine
  - time: "21:30"
    action: "on"
  - time: "21:45"
    action: "temp"
    temperature: "70F"   # Slightly warm for getting into bed
    
  # Sleep onset
  - time: "22:30"
    action: "temp" 
    temperature: "68F"   # Optimal sleep temperature
    
  # Deep sleep phase
  - time: "00:00"
    action: "temp"
    temperature: "65F"   # Cooler for deep sleep
    
  # REM sleep phase  
  - time: "03:00"
    action: "temp"
    temperature: "67F"   # Slightly warmer for REM
    
  # Pre-wake warming
  - time: "06:00"
    action: "temp"
    temperature: "72F"   # Warm up to ease waking
    
  # Wake time
  - time: "07:00"
    action: "off"        # Turn off after wake up
```

## Troubleshooting

### Check if daemon is running
```bash
ps aux | grep clim8
```

### View daemon logs (if running in background)
```bash
tail -f ~/clim8.log
```

### Test configuration
```bash
clim8 daemon --dry-run --verbose
```

### Common Issues

1. **No schedule items found**: Check your config file path and YAML syntax
2. **Authentication failed**: Verify your email and password in the config
3. **Actions not executing**: Ensure times are in correct format and check system time
4. **Permission denied**: Make sure config file is readable by the user running the daemon
5. **Device state out of sync after wake**: The daemon automatically detects and corrects this. If issues persist, check logs for sync warnings

### State Synchronization

The daemon automatically checks if your Eight Sleep device is in the correct state based on your schedule. This is especially useful when:

- Your computer goes to sleep/hibernation and misses scheduled events
- The device was manually controlled outside of the daemon
- Network connectivity was temporarily lost

The daemon will:
1. Determine what the device state should be based on the most recent scheduled action
2. Check the actual device state via the Eight Sleep API
3. Automatically correct any mismatches by executing the appropriate action

This feature can be disabled with `--sync-state=false` if needed.