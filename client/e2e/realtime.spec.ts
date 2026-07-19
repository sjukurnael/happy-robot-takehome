import { expect, test } from '@playwright/test'
import {
  column,
  createProjectViaUI,
  createTaskViaUI,
  deleteProjectViaAPI,
  dropConnection,
  newCollaborator,
  openProject,
  taskCard,
  uniqueName,
} from './helpers'

// The core promise of the system: changes made by one client appear on
// every other client's screen with no reload — driven by the WebSocket
// event broadcast and, after a disconnect, by seq-based replay. Each test
// runs two fully independent browser contexts (separate sessions,
// separate identities), the same as two people on two machines.
test.describe('real-time sync between two clients', () => {
  test('mutations propagate live: project, task, status, presence', async ({ browser, request }) => {
    const alice = await newCollaborator(browser, 'Alice')
    const bob = await newCollaborator(browser, 'Bob')
    const name = uniqueName('E2E Live')
    let id: string | null = null
    try {
      // Bob is already parked on the dashboard when Alice creates the
      // project — it must appear for him without any reload.
      await bob.page.goto('/')
      id = await createProjectViaUI(alice.page, name)
      await expect(bob.page.locator('.project-card', { hasText: name })).toBeVisible()

      await openProject(alice.page, name)
      await openProject(bob.page, name)

      // Presence: each side sees the other's avatar in the board header.
      await expect(alice.page.locator('.board-header-right .avatar[title="Bob"]')).toBeVisible()
      await expect(bob.page.locator('.board-header-right .avatar[title="Alice"]')).toBeVisible()

      // Alice creates a task; it appears on Bob's board untouched.
      await createTaskViaUI(alice.page, 'Live task')
      await expect(column(bob.page, 'To Do').locator('.task-card', { hasText: 'Live task' })).toBeVisible()

      // Alice moves it via the panel; Bob's card changes column.
      await taskCard(alice.page, 'Live task').click()
      await alice.page.locator('.panel .status-select').selectOption('in_progress')
      await expect(column(bob.page, 'In Progress').locator('.task-card', { hasText: 'Live task' })).toBeVisible()

      // Bob opens the same task — Alice's panel shows him watching.
      await taskCard(bob.page, 'Live task').click()
      await expect(alice.page.locator('.panel .watching-row .avatar[title="Bob"]')).toBeVisible()
    } finally {
      if (id) await deleteProjectViaAPI(request, id)
      await alice.context.close()
      await bob.context.close()
    }
  })

  test('comments sync live between two open panels', async ({ browser, request }) => {
    const alice = await newCollaborator(browser, 'Alice')
    const bob = await newCollaborator(browser, 'Bob')
    const name = uniqueName('E2E Comments')
    let id: string | null = null
    try {
      id = await createProjectViaUI(alice.page, name)
      await openProject(alice.page, name)
      await createTaskViaUI(alice.page, 'Discuss me')

      await bob.page.goto('/')
      await openProject(bob.page, name)
      await taskCard(alice.page, 'Discuss me').click()
      await taskCard(bob.page, 'Discuss me').click()

      await bob.page.locator('.panel input[placeholder^="Comment as"]').fill('Bob was here')
      await bob.page.getByRole('button', { name: 'Send' }).click()

      // Alice's open panel picks the comment up from the broadcast alone.
      await expect(alice.page.locator('.comment', { hasText: 'Bob was here' })).toBeVisible()
      await expect(alice.page.locator('.comment strong', { hasText: 'Bob' })).toBeVisible()
    } finally {
      if (id) await deleteProjectViaAPI(request, id)
      await alice.context.close()
      await bob.context.close()
    }
  })

  test('a disconnected client catches up on reconnect (seq replay)', async ({ browser, request }) => {
    const alice = await newCollaborator(browser, 'Alice')
    const bob = await newCollaborator(browser, 'Bob')
    const name = uniqueName('E2E Replay')
    let id: string | null = null
    try {
      id = await createProjectViaUI(alice.page, name)
      await openProject(alice.page, name)
      await bob.page.goto('/')
      await openProject(bob.page, name)
      await expect(alice.page.locator('.live-pill')).toBeVisible()

      // Alice drops off the network; the UI notices. setOffline blocks
      // new connections (so the 2s reconnect loop keeps failing while
      // offline) and dropConnection severs the current socket, which
      // Chromium's offline emulation leaves open.
      await alice.context.setOffline(true)
      await dropConnection(alice.page)
      await expect(alice.page.locator('.reconnecting-pill')).toBeVisible({ timeout: 15_000 })

      // Bob keeps working while Alice is gone.
      await createTaskViaUI(bob.page, 'Made while offline')
      await taskCard(bob.page, 'Made while offline').click()
      await bob.page.locator('.panel .status-select').selectOption('in_progress')
      await bob.page.locator('.panel-close').click()

      // Alice reconnects: the client re-sends "viewing" with its lastSeq
      // and the server replays exactly the missed events — the task must
      // appear, already In Progress, with no reload.
      await alice.context.setOffline(false)
      await expect(alice.page.locator('.live-pill')).toBeVisible({ timeout: 15_000 })
      await expect(
        column(alice.page, 'In Progress').locator('.task-card', { hasText: 'Made while offline' }),
      ).toBeVisible({ timeout: 15_000 })
    } finally {
      if (id) await deleteProjectViaAPI(request, id)
      await alice.context.close()
      await bob.context.close()
    }
  })
})
