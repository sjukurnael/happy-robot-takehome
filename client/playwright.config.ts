import { defineConfig, devices } from '@playwright/test'

// The suite drives the real composed stack (nginx-served client + Go
// server + Postgres) — run `make up` first, or point E2E_BASE_URL at a
// running deployment. Tests create their own uniquely-named projects and
// delete them, so they can safely share a database with dev/seed data.
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:3000',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
