import { expect, test } from '@playwright/test'
import { column, createProjectViaUI, deleteProjectViaAPI, newCollaborator, openProject, uniqueName } from './helpers'

test.describe('project lifecycle', () => {
  test('create, open, and delete a project', async ({ browser, request }) => {
    const { context, page } = await newCollaborator(browser, 'Pat')
    const name = uniqueName('E2E Lifecycle')
    const id = await createProjectViaUI(page, name, 'made by the e2e suite')
    try {
      const card = page.locator('.project-card', { hasText: name })
      await expect(card).toContainText('made by the e2e suite')

      // A fresh project opens to an empty three-column board.
      await openProject(page, name)
      for (const label of ['To Do', 'In Progress', 'Done'] as const) {
        await expect(column(page, label).locator('.empty-hint')).toHaveText('No tasks')
      }

      // Back to the dashboard, then delete through the UI (confirm() is
      // auto-accepted by the helper's dialog handler).
      await page.getByRole('button', { name: '← Projects' }).click()
      await card.locator('.delete-btn').click()
      await expect(card).not.toBeVisible()
    } finally {
      await deleteProjectViaAPI(request, id)
      await context.close()
    }
  })

  test('search filters the project grid', async ({ browser, request }) => {
    const { context, page } = await newCollaborator(browser, 'Pat')
    const needle = uniqueName('E2E Needle')
    const hay = uniqueName('E2E Hay')
    const needleId = await createProjectViaUI(page, needle)
    const hayId = await createProjectViaUI(page, hay)
    try {
      await page.getByPlaceholder('Search projects…').fill(needle)
      await expect(page.locator('.project-card', { hasText: needle })).toBeVisible()
      await expect(page.locator('.project-card', { hasText: hay })).not.toBeVisible()
    } finally {
      await deleteProjectViaAPI(request, needleId)
      await deleteProjectViaAPI(request, hayId)
      await context.close()
    }
  })
})
