import { test, expect } from '@playwright/test';

test('app loads and navigates', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('nav')).toBeVisible();

  await page.click('text=搜索');
  await expect(page).toHaveURL(/\/search/);
});

test('search page has search input', async ({ page }) => {
  await page.goto('/search');
  await expect(page.locator('input[type="text"]')).toBeVisible();
});

test('404 page works', async ({ page }) => {
  await page.goto('/nonexistent');
  await expect(page.locator('text=404')).toBeVisible();
});
