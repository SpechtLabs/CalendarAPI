---
title: Quick Start Guide
permalink: /guide/quickstart
createTime: 2025/05/29 22:10:58
---

This guide walks you through installing CalendarAPI as a [Home Assistant Add-On](../config/home_assistant.md), configuring it with an external iCal calendar URL, and setting up a basic status rule and automation script.

## 1. Add the SpechtLabs Add-On Repository

In Home Assistant:

1. Go to **Settings → Add-ons → Add-on Store**.
2. Click the **three-dot menu** (⋮) in the top-right corner and select **Repositories**.
3. Add the following URL:

```plaintext
https://github.com/SpechtLabs/homeassistant-addons
```

4. Click **Add**, then scroll down to find the **CalendarAPI** add-on.

<!-- Screenshot Placeholder: Add-on repository added -->

## 2. Install and Start the CalendarAPI Add-On

1. Click on **CalendarAPI** from the list of available add-ons.
2. Click **Install**.
3. Once installed, go to the **Configuration** tab.

You will configure your calendar source and optional rules before starting the service.

<!-- Screenshot Placeholder: Add-on configuration screen -->

## 3. Configure Your Calendar Source

In the configuration editor, define your calendar source using an external iCal URL.

Example configuration:

```yaml
calendars:
  - name: "work"
    url: "https://calendar.google.com/calendar/ical/example%40gmail.com/private-abcdefg/basic.ics"
```

This will make your calendar available under the name `work`.

## 4. Add a Rule to Filter Events

You can define rules to filter or relabel events based on keywords. Below is an example that renames all events containing "Standup" to a custom title:

```yaml
rules:
  - name: "Normalize Standup Title"
    key: "title"
    contains:
      - "Standup"
    relabelConfig:
      message: "Daily Sync"
      important: true

```

This makes it easier to normalize calendar event data for display or automation.

## 5. Save and Start the Add-On

1. After editing the configuration, click **Save**.
2. Return to the **Info** tab and click **Start** to launch the CalendarAPI service.
3. Optionally enable **Start on boot**.

<!-- Screenshot Placeholder: Add-on started and running -->

## 6. Set a Custom "Do Not Disturb" Status from Home Assistant

You can define a script in Home Assistant to trigger a custom status via CalendarAPI.

Add this to your `configuration.yaml`:

```yaml
rest_command:
  set_dnd_status:
    url: http://192.168.0.62:8099/status
    method: post
    content_type: application/json
    payload: >
      {
        "calendar_name": "work",
        "status": {
          "icon": "warning_icon",
          "icon_size": 196,
          "title": "Do Not Disturb",
          "description": "Currently unavailable"
        }
      }

  clear_dnd_status:
    url: http://192.168.0.62:8099/status
    method: delete
    content_type: application/json
    payload: '{ "calendar_name": "all" }'
```

Now you can call the `rest_command` from abritraty automations or scripts.

```yaml
script:
  do_not_disturb:
    alias: "Set Do Not Disturb"
    sequence:
      - service: rest_command.set_dnd_status
```

## Done

You can now extend this setup with [REST sensors](../config/home_assistant.md#sensor-configuration), or [sending custom status updates](../config/home_assistant.md#sending-custom-status-updates) via automations as needed.
