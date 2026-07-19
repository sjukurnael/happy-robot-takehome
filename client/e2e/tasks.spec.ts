import { expect, test } from '@playwright/test'
import {
  column,
  createProjectViaUI,
  createTaskViaUI,
  deleteProjectViaAPI,
  newCollaborator,
  openProject,
  taskCard,
  uniqueName,
} from './helpers'

test.describe('task lifecycle', () => {
  test('create, edit, comment on, and delete a task', async ({ browser, request }) => {
    const { context, page } = await newCollaborator(browser, 'Pat')
    const name = uniqueName('E2E Tasks')
    const id = await createProjectViaUI(page, name)
    try {
      await openProject(page, name)
      await createTaskViaUI(page, 'Ship the widget', { description: 'End to end', tags: 'e2e, widget' })

      // Lands in To Do with its priority and tags rendered.
      const card = taskCard(page, 'Ship the widget')
      await expect(column(page, 'To Do').locator('.task-card', { hasText: 'Ship the widget' })).toBeVisible()
      await expect(card.locator('.priority-badge')).toHaveText('medium')
      await expect(card.locator('.tag')).toHaveText(['e2e', 'widget'])

      // Status change through the panel moves the card between columns.
      await card.click()
      await page.locator('.panel .status-select').selectOption('in_progress')
      await expect(column(page, 'In Progress').locator('.task-card', { hasText: 'Ship the widget' })).toBeVisible()

      // Comment thread.
      await page.locator('.panel input[placeholder^="Comment as"]').fill('Looks good to me')
      await page.getByRole('button', { name: 'Send' }).click()
      await expect(page.locator('.comment', { hasText: 'Looks good to me' })).toBeVisible()
      await expect(page.locator('.panel h3')).toHaveText('Comments · 1')

      // Delete the task from the panel (confirm() auto-accepted).
      await page.locator('.panel .delete-btn').click()
      await expect(card).not.toBeVisible()
    } finally {
      await deleteProjectViaAPI(request, id)
      await context.close()
    }
  })

  test('a task cannot be completed while its dependency is', async ({ browser, request }) => {
    const { context, page } = await newCollaborator(browser, 'Pat')
    const name = uniqueName('E2E Deps')
    const id = await createProjectViaUI(page, name)
    try {
      await openProject(page, name)
      await createTaskViaUI(page, 'Build API')
      await createTaskViaUI(page, 'Deploy', { dependsOn: ['Build API'] })

      const deploy = taskCard(page, 'Deploy')
      await expect(deploy.locator('.blocked-badge')).toBeVisible()

      // Completing a blocked task is rejected by the server (409) and the
      // panel surfaces the error; the task stays in To Do.
      await deploy.click()
      await page.locator('.panel .status-select').selectOption('done')
      await expect(page.locator('.panel .error-banner')).toBeVisible()
      await page.locator('.panel-close').click()
      await expect(column(page, 'To Do').locator('.task-card', { hasText: 'Deploy' })).toBeVisible()

      // Complete the dependency; the blocked badge clears and Deploy can
      // now be completed.
      await taskCard(page, 'Build API').click()
      await page.locator('.panel .status-select').selectOption('done')
      await page.locator('.panel-close').click()
      await expect(deploy.locator('.blocked-badge')).not.toBeVisible()

      await deploy.click()
      await page.locator('.panel .status-select').selectOption('done')
      await page.locator('.panel-close').click()
      await expect(column(page, 'Done').locator('.task-card', { hasText: 'Deploy' })).toBeVisible()
    } finally {
      await deleteProjectViaAPI(request, id)
      await context.close()
    }
  })

  test('drag and drop moves a task between columns', async ({ browser, request }) => {
    const { context, page } = await newCollaborator(browser, 'Pat')
    const name = uniqueName('E2E Drag')
    const id = await createProjectViaUI(page, name)
    try {
      await openProject(page, name)
      await createTaskViaUI(page, 'Drag me')

      const card = taskCard(page, 'Drag me')
      const target = column(page, 'In Progress').locator('.column-body')
      const cardBox = (await card.boundingBox())!
      const targetBox = (await target.boundingBox())!

      // dnd-kit's PointerSensor needs real pointer movement (4px
      // activation distance), so drive the mouse manually in steps.
      const patchDone = page.waitForResponse(
        (r) => r.request().method() === 'PATCH' && r.url().includes('/api/tasks/'),
      )
      await page.mouse.move(cardBox.x + cardBox.width / 2, cardBox.y + cardBox.height / 2)
      await page.mouse.down()
      await page.mouse.move(targetBox.x + targetBox.width / 2, targetBox.y + targetBox.height / 2, { steps: 12 })
      await page.mouse.up()

      await expect(column(page, 'In Progress').locator('.task-card', { hasText: 'Drag me' })).toBeVisible()
      // The optimistic move must survive the server round trip: wait for
      // the PATCH to land (reloading mid-flight would race the commit),
      // then reload and confirm the status was actually persisted. The
      // app is a routerless SPA — a reload lands on the dashboard, so the
      // project has to be reopened to see the board again.
      expect((await patchDone).ok()).toBe(true)
      await page.reload()
      await openProject(page, name)
      await expect(column(page, 'In Progress').locator('.task-card', { hasText: 'Drag me' })).toBeVisible()
    } finally {
      await deleteProjectViaAPI(request, id)
      await context.close()
    }
  })
})
