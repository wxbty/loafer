import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: 'http://xxx.72.42.149:8080',
    trace: 'on-first-retry',
    screenshot: 'on',
    video: 'off',
    channel: 'chromium',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        launchOptions: {
          executablePath: '/usr/bin/chromium-browser',
          args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage'],
        },
      },
    },
  ],
});
