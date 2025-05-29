---
title: CLI & Server Usage
createTime: 2025/04/01 00:08:53
permalink: /guide/usage
---

The `calendarapi` binary can run in two distinct modes:

1. As a **server**, exposing calendar data over REST and gRPC
2. As a **CLI client**, allowing you to interact with an existing CalendarAPI server

This section covers both modes and explains the available commands.

## Server Mode: `calendarapi serve`

To start a CalendarAPI server, use the `serve` command:

```bash
calendarapi serve
```

This will read your configured iCal calendars from the calendar config and expose them over both REST and gRPC interfaces. The ports and bindings are defined in your server configuration.

This is the typical setup when running CalendarAPI as a long-lived backend service.

## Client Mode: `calendarapi [command] [resource]`

You can also use calendarapi as a command-line client to interact with a running CalendarAPI server.

This mode is useful for:

Fetching upcoming calendar events
Setting or clearing status messages
Integrating CalendarAPI into scripts or automations
You can either set the server address in the configuration file, or pass it directly via --server:

```bash
calendarapi get calendar --server http://localhost:8080
```

### Command Structure

The CLI follows a verb noun structure, similar to tools like kubectl:

```bash
calendarapi [command] [resource]
```

#### Available Commands

<FileTree>

- get
  - status
  - calendar
- clear
  - status
  - calendar
- set
  - status

</FileTree>

You can explore these interactively using:

```bash
calendarapi --help
```

#### Tab Completion

`calendarapi` supports shell tab-completion for all major shells:

- Bash
- Zsh
- Fish
- PowerShell

To enable Zsh completion, for example, add this to your `~/.zshrc`:

```zsh
if [[ $(command -v calendarapi) ]]; then
  eval "$(calendarapi completion zsh)"
fi
```

Then reload your shell:

```zsh
source ~/.zshrc
```
