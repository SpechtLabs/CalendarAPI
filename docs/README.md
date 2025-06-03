---
pageLayout: home
externalLinkIcon: false

config:
  - type: doc-hero
    hero:
      name: CalendarAPI
      text: Easily expose your calendars via modern APIs.
      tagline: CalendarAPI parses iCal (.ics) files and serves them over gRPC and REST. It supports hot config reloads via Viper and comes with a Home Assistant add-on.
      image: /logo.png
      actions:
        - text: Get Started →
          link: /guide/overview
          theme: brand
          icon: simple-icons:bookstack
        - text: GitHub Releases →
          link: https://github.com/SpechtLabs/CalendarAPI/releases
          theme: alt
          icon: simple-icons:github

  - type: features
    title: Why CalendarAPI?
    description: A modern, hackable API layer for your calendars.
    features:
      - title: Parse iCal (.ics) files from URLs or local paths
        icon: mdi:file-document-check-outline
        details: CalendarAPI reads `.ics` files directly from remote or local sources, keeping your workflow flexible.

      - title: Exposes events via REST and gRPC APIs
        icon: mdi:api
        details: Modern API endpoints make it easy to query and integrate calendar data into any stack.

      - title: Rule engine for filtering and relabeling events
        icon: mdi:filter-settings
        details: A powerful built-in engine to filter, relabel, and skip events based on your custom rules.

      - title: Hot config reloads via Viper
        icon: mdi:reload
        details: No restarts needed — changes to your config are picked up live using Viper.

      - title: Home Assistant Add-On
        icon: mdi:home-assistant
        details: Easily deploy CalendarAPI into your smart home using the official HomeAssistant Add-On.

      - title: Custom status messages for displays
        icon: mdi:message-badge-outline
        details: Set dynamic status messages per calendar — perfect for e-Paper displays or presence indicators.

      - title: CLI client for querying and scripting
        icon: mdi:console
        details: Use the CLI to fetch calendars, set statuses, and integrate CalendarAPI into your shell workflows.

      - title: Minimal footprint, deploy anywhere
        icon: mdi:cloud-outline
        details: Lightweight Go binary runs on containers, Raspberry Pi, or any Linux host with minimal resources.


  - type: VPReleasesCustom
    repo: SpechtLabs/CalendarAPI

  - type: VPContributorsCustom
    repo: SpechtLabs/CalendarAPI
---
