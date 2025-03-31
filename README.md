# 📆 CalendarAPI

CalendarAPI is a service that parses iCal files and exposes their content via gRPC or a REST API. It uses Viper for configuration, which supports runtime reloads.

## ⚙️ Features

- ✅ Parse iCal (.ics) files from **URLs or local files**
- ✅ Exposes events via **REST** and **gRPC** APIs
- ✅ Built-in **rule engine** for relabeling, filtering, and skipping events
- ✅ Supports **hot configuration reloads** (with [Viper](https://github.com/spf13/viper))
- ✅ [HomeAssistant Add-On] to easily host CalendarAPI on your Home Assistant

## Configuration

### Server config

```yaml
server:
  server: ""      # ! Does not support changing at runtime
  httpPort: 8080  # ! Does not support changing at runtime
  grpcPort: 50051 # ! Does not support changing at runtime
  debug: false    # Control if debug logs are printed
  refresh: 5m     # how often are the calendars refreshed? Must be parsable by time.ParseDuration()
```

### Calendar config

It is possible to add multiple iCal calendar feeds to pull multiple calendars from multiple sources.
ical files can be read either from a URL or from a local file

```yaml
calendars:
  calendar1:
    from: file
    ical: ./path/to/calender1.ics
  calendar2:
    from: url
    ical: www.example.com/calendar/calendar.ics
```

### Rules config

Rules allow you to filter events, or apply re-labeling.
Rules are evaluated in the order of which they are specified in.
The rule-evaluation stops after the event matches the first rule.

#### Relabel

The rule below matches if the `Title` of your calendar event contains the string `1:1` and then sets the display message to `1:1` as well as marking the event as important

```yaml
rules:
  - name: "1:1s"
    key: "title"
    contains:
      - "1:1"
    relabelConfig:
      message: "1:1"
      important: true
```

#### Skip rules

If a rule specifies `skip: true` then each calendar event that matches this rule is excluded from the API responses.
Below are examples to skip all day and non-blocking events:

```yaml
rules:
  - name: "Skip all day events"
    key: "all_day"
    contains:
      - "true"
    skip: true

  - name: "Skip non-blocking events"
    key: "busy"
    contains:
      - "Free"
    skip: true
```

#### Wildcard matching

You can use `*` as a wildcard to match everything. If `key` uses the wildcard, it will search all fields.
This is useful for catch-all rules that allow all other events that did not match previous rules to be included in the API responses.

```yaml
rules:
  - name: "Allow everything else"
    key: "*"
    important: false
    contains:
      - "*"
```

## Development

### Prerequisites

- Go 1.24+
- Docker (for building and running containers)

### Building

To build the project, run:

```sh
go build -o calendarapi main.go
```

### Running

To run the project, execute:

```sh
./calendarapi
```

### Docker

To build and run the Docker container, use the following commands:

```sh
docker build -t calendarapi .
docker run -p 8080:8080 -p 50051:50051 calendarapi
```

### Testing

To run tests, use:

```sh
go test ./...
```

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

[HomeAssistant Add-On]: https://github.com/SpechtLabs/homeassistant-addons/tree/main/calendar_api
