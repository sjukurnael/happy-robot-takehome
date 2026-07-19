import { expect, test } from '@playwright/test'
import {
  createProjectViaUI,
  createTaskViaUI,
  deleteProjectViaAPI,
  newCollaborator,
  openProject,
  taskCard,
  uniqueName,
} from './helpers'

// CI (and the default `make up` stack) runs without ANTHROPIC_API_KEY, so
// this suite exercises the AI breakdown's designed degraded path: the
// button is present, the modal opens, the server's 503 message is surfaced
// verbatim in the error banner, and dismissing leaves the board untouched.
// The happy path (a real Claude call) is deliberately not e2e-tested —
// it's nondeterministic and needs a paid key; the apply endpoint's
// correctness is covered by the Go integration tests.
test.describe('AI breakdown (no API key configured)', () => {
  test('surfaces the not-configured error and dismisses cleanly', async ({ browser, request }) => {
    const { context, page } = await newCollaborator(browser, 'Pat')
    const name = uniqueName('E2E Breakdown')
    const id = await createProjectViaUI(page, name)
    try {
      await openProject(page, name)
      await createTaskViaUI(page, 'Launch the rocket')

      await taskCard(page, 'Launch the rocket').click()
      await page.locator('.panel .breakdown-btn').click()

      const modal = page.locator('.breakdown-modal')
      await expect(modal).toBeVisible()
      await expect(modal.locator('.error-banner')).toContainText('AI breakdown is not configured')
      // With no suggestions there is nothing to submit, but retry is offered.
      await expect(modal.getByRole('button', { name: /Create \d+ subtask/ })).toBeDisabled()
      await expect(modal.getByRole('button', { name: 'Retry' })).toBeVisible()

      // Dismissing returns to the still-open task panel; no tasks appeared.
      await modal.getByRole('button', { name: 'Cancel' }).click()
      await expect(modal).not.toBeVisible()
      await expect(page.locator('.panel')).toBeVisible()
      await expect(page.locator('.task-card')).toHaveCount(1)
    } finally {
      await deleteProjectViaAPI(request, id)
      await context.close()
    }
  })
})
