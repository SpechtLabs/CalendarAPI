import { viteBundler } from '@vuepress/bundler-vite';
import { registerComponentsPlugin } from '@vuepress/plugin-register-components';
import { path } from '@vuepress/utils';
import { defineUserConfig } from 'vuepress';
import { plumeTheme } from 'vuepress-theme-plume';

export default defineUserConfig({
  base: '/',
  lang: 'en-US',
  title: 'Calendar API',
  description: 'Easily access your calendars via gRPC or REST.',

  head: [
    ['meta', { name: "description", content: "CalendarAPI is a service that parses iCal files and exposes their content via gRPC or a REST API." }],
    ['link', { rel: 'icon', type: 'image/png', href: '/images/specht.png' }],
  ],

  bundler: viteBundler(),
  shouldPrefetch: false,

  plugins: [
    registerComponentsPlugin({
      componentsDir: path.resolve(__dirname, './components'),
    }),
  ],

  theme: plumeTheme({
    docsRepo: 'https://github.com/SpechtLabs/CalendarAPI',
    docsDir: 'docs',
    docsBranch: 'main',

    editLink: false,
    lastUpdated: false,
    contributors: false,

    cache: 'filesystem',
    search: { provider: 'local' },

    // The docs are two sections of pages; the navbar links each page too.
    sidebar: {
      '/guide/': [
        {
          text: 'Getting Started',
          collapsed: false,
          prefix: '/guide/',
          items: [
            { text: 'Overview', link: 'overview' },
            { text: 'Quick Start', link: 'quickstart' },
            { text: 'CLI & Server Usage', link: 'usage' },
          ],
        },
      ],
      '/config/': [
        {
          text: 'Configuration',
          collapsed: false,
          prefix: '/config/',
          items: [
            { text: 'Server', link: 'server' },
            { text: 'Calendars', link: 'calendars' },
            { text: 'Rules Engine', link: 'rules' },
            { text: 'Home Assistant Add-On', link: 'home_assistant' },
          ],
        },
      ],
    },

    /**
      * markdown
      * @see https://theme-plume.vuejs.press/config/markdown/
      */
    markdown: {
      collapse: true,
      timeline: true,
      mermaid: true,
      // The theme turns math on by default and logs an error without a
      // renderer installed; no page has formulas.
      math: false,
      image: {
        figure: true,
        lazyload: true,
        mark: true,
        size: true,
      },
    },

    watermark: false,
  }),
})
