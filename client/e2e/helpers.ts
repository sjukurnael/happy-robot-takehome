import { expect, type APIRequestContext, type Browser, type BrowserContext, type Page } from '@playwright/test'

export interface Collaborator {
  context: BrowserContext
  page: Page
}

// Opens a fresh browser context pre-seeded with a taskman identity, so the
// app's first-run window.prompt never fires, and with confirm() dialogs
// (project/task deletion) auto-accepted.
export async function newCollaborator(browser: Browser, name: string): Promise<Collaborator> {
  const context = await browser.newContext()
  await context.addInitScript(
    (identity) => {
      // Guard so a navigation doesn't mint a new clientId mid-test — the
      // app expects a tab's identity to be stable (sessionStorage).
      if (!sessionStorage.getItem('taskman.identity')) {
        sessionStorage.setItem('taskman.identity', identity)
      }
      // Track live WebSockets so dropConnection() can sever them:
      // context.setOffline() blocks new connections but does NOT close an
      // already-established WebSocket in Chromium, so simulating a real
      // network drop needs both.
      const sockets: WebSocket[] = []
      ;(window as unknown as { __sockets: WebSocket[] }).__sockets = sockets
      const OrigWebSocket = window.WebSocket
      window.WebSocket = class extends OrigWebSocket {
        constructor(url: string | URL, protocols?: string | string[]) {
          super(url, protocols)
          sockets.push(this)
        }
      }
    },
    JSON.stringify({ clientId: crypto.randomUUID(), name }),
  )
  const page = await context.newPage()
  page.on('dialog', (dialog) => dialog.accept())
  return { context, page }
}

// Severs the page's live WebSocket(s). Use together with
// context.setOffline(true) to simulate a genuine network drop (see the
// note in newCollaborator's init script).
export async function dropConnection(page: Page): Promise<void> {
  await page.evaluate(() => {
    ;(window as unknown as { __sockets: WebSocket[] }).__sockets.forEach((ws) => ws.close())
  })
}

export function uniqueName(prefix: string): string {
  return `${prefix} ${Date.now()}-${Math.floor(Math.random() * 1e6)}`
}

// Creates a project through the real UI (dashboard form) and waits for its
// card to appear. Returns the project's id, looked up via the API so tests
// can clean up after themselves regardless of how they end.
export async function createProjectViaUI(page: Page, name: string, description = ''): Promise<string> {
  await page.goto('/')
  await page.getByPlaceholder('Project name').fill(name)
  if (description) await page.getByPlaceholder('Description').fill(description)
  await page.getByRole('button', { name: 'Create project' }).click()
  await expect(page.locator('.project-card', { hasText: name })).toBeVisible()

  const res = await page.request.get('/api/projects/')
  const projects = (await res.json()) as { id: string; name: string }[]
  const project = projects.find((p) => p.name === name)
  if (!project) throw new Error(`project "${name}" not found via API after UI creation`)
  return project.id
}

// API-level cleanup — used in afterEach/finally so a failed test never
// leaves its fixture project behind in the shared database.
export async function deleteProjectViaAPI(request: APIRequestContext, projectId: string): Promise<void> {
  await request.delete(`/api/projects/${projectId}/`)
}

export async function openProject(page: Page, name: string): Promise<void> {
  await page.locator('.project-card', { hasText: name }).click()
  await expect(page.locator('h1', { hasText: name })).toBeVisible()
}

// Creates a task through the "+ New task" modal on an open board.
export async function createTaskViaUI(
  page: Page,
  title: string,
  opts: { description?: string; tags?: string; dependsOn?: string[] } = {},
): Promise<void> {
  await page.getByRole('button', { name: '+ New task' }).click()
  await page.getByPlaceholder('Task title').fill(title)
  if (opts.description) await page.locator('.modal textarea').fill(opts.description)
  if (opts.tags) await page.getByPlaceholder('frontend, urgent').fill(opts.tags)
  for (const dep of opts.dependsOn ?? []) {
    await page.locator('.dep-toggle-chip', { hasText: dep }).click()
  }
  await page.getByRole('button', { name: 'Create task' }).click()
  await expect(taskCard(page, title)).toBeVisible()
}

export function column(page: Page, label: 'To Do' | 'In Progress' | 'Done') {
  return page.locator('.column', { has: page.locator('.column-label', { hasText: label }) })
}

export function taskCard(page: Page, title: string) {
  return page.locator('.task-card', { hasText: title })
}
