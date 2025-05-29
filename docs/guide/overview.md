---
title: Overview
createTime: 2025/04/01 00:08:53
permalink: /guide/overview
---

## Background

**CalendarAPI** was originally created to power an e-Paper-based meeting room display running on an ESP32.
Parsing `.ics` calendar files directly on a memory-constrained microcontroller (in Arduino C++) would have been impractical, so CalendarAPI offloaded that logic into a dedicated Go service.

This service parses iCal files and exposes the resulting event data over **REST** and **gRPC** APIs, making it easy to consume calendar data from modern applications.

## Features

What started as a backend utility quickly evolved into a more versatile tool:

- A **CLI** to check upcoming calendar events via `calendarapi get calendar`.
- A [**Home Assistant Add-On**](https://github.com/SpechtLabs/homeassistant-addons/tree/main/calendar_api) for seamless smart home integration.
- A backend for [**RESTful sensors**](https://www.home-assistant.io/integrations/sensor.rest/) and [**RESTful commands**](https://www.home-assistant.io/integrations/rest_command/) in Home Assistant.

CalendarAPI also supports **custom status messages per calendar**, allowing displays to show dynamic content throughout the day — such as _“In a meeting”_ or _“Out for lunch.”_

At its core, CalendarAPI is designed to make calendar data:

- **Accessible** across systems and devices
- **Automatable** through reliable APIs
- **Adaptable** to embedded platforms and home automation environments

## Real-World Use Cases

Here are some practical rules and configurations that users like you are already using in the wild:

### 🧼 Filtering Noise: Skip Unwanted Events

Exclude irrelevant entries like all-day events or office hours.

```yaml
- name: Remove all-day events from API
  key: all_day
  contains:
    - "true"
  skip: true

- name: Remove non-blocking and OOO from API
  key: busy
  contains:
    - Free
    - OutOfOffice
  skip: true
```

### Highlight 1:1 Meetings

```yaml
- name: 1:1s
  key: title
  contains:
    - "1:1"
  message: "1:1"
  important: true
```

### Normalize Scrum Meeting title

```yaml
- name: Scrum Meetings
  key: title
  message: Scrum Meeting
  contains:
    - Scrum
    - Planning
    - Retrospective
    - Sprint Review
    - Grooming
  important: false
```

### Label Internal Syncs

```yaml
- name: Important Team Meetings
  key: title
  contains:
    - Townhall
  message: Team Meeting
  important: true
```

### Catch-All Rule

Ensure unmatched events are still returned by default:

```yaml
- name: Catch All
  key: "*"
  important: false
  contains:
    - "*"
```
