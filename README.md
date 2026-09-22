<p align="center">
  <a href="https://github.com/blacktop/clim8"><img alt="clim8 Logo" src="https://raw.githubusercontent.com/blacktop/clim8/main/docs/logo.webp" width="800" /></a>
  <h2><p align="center">Control Eight Sleep via CLI</p></h2>
  <p align="center">
    <a href="https://github.com/blacktop/clim8/actions" alt="Actions">
          <img src="https://github.com/blacktop/clim8/actions/workflows/go.yml/badge.svg" /></a>
    <a href="https://github.com/blacktop/clim8/releases/latest" alt="Downloads">
          <img src="https://img.shields.io/github/downloads/blacktop/clim8/total.svg" /></a>
    <a href="https://github.com/blacktop/clim8/releases" alt="GitHub Release">
          <img src="https://img.shields.io/github/release/blacktop/clim8.svg" /></a>
    <a href="http://doge.mit-license.org" alt="LICENSE">
          <img src="https://img.shields.io/:license-mit-blue.svg" /></a>
</p>
<br>

## Why? 🤔

Oh, I think you know why 😏

## Getting Started

### Install

```bash
go install github.com/blacktop/clim8@latest
```

Or via [homebrew](https://brew.sh)

```bash
brew install blacktop/tap/clim8
```

Or download the latest [release](https://github.com/blacktop/clim8/releases/latest)

### Running

```bash
❱ clim8 --help
```
```bash
Eight Sleep CLI

Usage:
  clim8 [command]

Available Commands:
  alarm       Manage Eight Sleep alarms
  away        Pause the pod while you travel
  daemon      Run Eight Sleep scheduler daemon
  feats       Dump release features
  help        Help about any command
  info        Show Eight Sleep Info
  off         Turn off Eight Sleep Pod
  on          Turn on Eight Sleep Pod
  prime       Start a priming cycle
  sleep       Show sleep data for a day
  status      Show Eight Sleep status
  temp        Set the temperature of Eight Sleep Pod
  tracks      List audio tracks
  version     Show version number

Flags:
  -e, --email string      Email address
  -h, --help              help for clim8
      --json              Output JSON instead of text
  -p, --password string   Password
  -V, --verbose           Enable verbose debug logging

Use "clim8 [command] --help" for more information about a command.
```

### Config

Set your config `~/.config/clim8/config.yaml`

```yaml
# Your Eight Sleep credentials
email: your@email.com
password: your-password

# Schedule for automated control (optional)
schedule:
  - time: "22:00"    # 10:00 PM - Turn on
    action: "on"
  - time: "22:15"    # 10:15 PM - Set temperature  
    action: "temp"
    temperature: "68F"
  - time: "01:00"    # 1:00 AM - Lower temp for deep sleep
    action: "temp"
    temperature: "65F"
  - time: "06:00"    # 6:00 AM - Turn off
    action: "off"
  - time: "22:30"    # Optional: control another side with left, right or both
    action: "on"
    side: "right"
```

### Manual Commands

You can also control your Eight Sleep pod manually:

```bash
# Turn on the pod
clim8 on

# Set temperature (turns on automatically)
clim8 temp 68F

# Turn off the pod
clim8 off

# Check status: both sides, water level, priming, hub health
clim8 status
clim8 status --json

# Control the other side of the bed, or both
clim8 on --side both
clim8 temp 66F --side right

# Hold a temperature for a while; the side then turns off unless something else is scheduled
clim8 temp 72F --for 2h

# Change an Autopilot level (bedtime, initial or final) without applying it now
clim8 temp 66F --stage final

# Pause a side while travelling (shown as AWAY in `clim8 status`)
clim8 away on
clim8 away off

# Alarms
clim8 alarm list
clim8 alarm create 07:30 --vibration 60
clim8 alarm disable <id>
clim8 alarm snooze --minutes 9   # the alarm that is ringing
clim8 alarm dismiss

# Last night's sleep
clim8 sleep
clim8 sleep --date 2026-09-20 --json

# Start a priming cycle
clim8 prime
```

Commands act on your own side unless you pass `--side left`, `--side right` or `--side both`.
Alarms and sleep data belong to one person, so those take `left` or `right` only.

### Daemon Scheduler

The `daemon` command runs a background scheduler that automatically controls your Eight Sleep pod based on your configured schedule.

```bash
# Run daemon manually
clim8 daemon

# Test your schedule without executing actions
clim8 daemon --dry-run

# Install as system service via homebrew
brew services start blacktop/tap/clim8
```

The daemon will:
- Execute actions at scheduled times throughout the night
- Automatically restart if it crashes
- Log all activity to `/usr/local/var/log/clim8.log`
- Run continuously in the background

📖 **[Full daemon documentation and examples →](docs/daemon.md)**

## License

MIT Copyright (c) 2025 **blacktop**