import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

test("complete audit workflow persists dispositions, replays evidence and simulates rules", async ({
  page,
}) => {
  const fixture = JSON.parse(
    readFileSync("../test/fixtures/population.json", "utf8"),
  ) as Record<string, unknown>;
  const population = { ...fixture, source_id: `e2e-${Date.now()}` };
  const importPopulation = async () => {
    await page
      .getByRole("button", { name: "Sources & mapping", exact: true })
      .click();
    await page
      .getByLabel("Choose extract file")
      .setInputFiles({
        name: "population.json",
        mimeType: "application/json",
        buffer: Buffer.from(JSON.stringify(population)),
      });
    await page.getByRole("button", { name: "Validate & queue run" }).click();
    await expect(
      page.getByRole("heading", { name: "Run monitor" }),
    ).toBeVisible();
    await expect(page.locator("tbody tr").first()).toContainText("Completed", {
      timeout: 45000,
    });
  };
  const openQueue = async () => {
    await page
      .getByRole("button", { name: "Exception queue", exact: true })
      .click();
    await page
      .getByPlaceholder("Search record or exception ID")
      .fill(population.source_id);
  };
  const errors: string[] = [];
  const external: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("request", (r) => {
    if (
      !r.url().startsWith("http://localhost:8088") &&
      !r.url().startsWith("data:") &&
      !r.url().startsWith("blob:")
    )
      external.push(r.url());
  });
  await page.goto("/");
  await page
    .getByLabel("Workspace access token")
    .fill(readFileSync("../secrets/demo_token", "utf8").trim());
  await page.getByRole("button", { name: "Open workspace" }).click();
  await expect(
    page.getByRole("heading", { name: "Exception queue" }),
  ).toBeVisible();
  await importPopulation();
  await openQueue();
  await expect(page.locator("tbody tr")).toHaveCount(4);
  await page.screenshot({
    path: "../dist/screenshots/queue.png",
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "P1 + P2", exact: false })
    .first()
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button", { name: "Verify replay" }).first().click();
  await expect(page.getByRole("status")).toContainText("Replay verified");
  await page
    .getByLabel("Reason or investigation note")
    .fill("Verified distinct settlement references in the synthetic fixture.");
  await page.getByRole("button", { name: "Start review", exact: true }).click();
  await page
    .getByLabel("Reason or investigation note")
    .fill("Both synthetic payments are legitimate.");
  await page.getByRole("button", { name: "Dismiss", exact: true }).click();
  await expect(page.locator(".detail-badges")).toContainText("Dismissed");
  await page.getByRole("button", { name: "Close detail" }).click();
  await importPopulation();
  await openQueue();
  await expect(page.locator("tbody tr")).toHaveCount(3);
  await page.getByRole("button", { name: "Dismissed", exact: true }).click();
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await page.getByRole("button", { name: "Rule catalog", exact: true }).click();
  await page.getByRole("button", { name: /AP-01.*Duplicate payments/ }).click();
  await page.getByRole("button", { name: "Simulate draft" }).click();
  await expect(page.getByRole("status")).toContainText(
    "All 3 required fixtures passed",
  );
  await page.getByRole("button", { name: "Open decision graph" }).click();
  await expect(page.locator(".graph-editor")).toBeVisible();
  await page.waitForTimeout(1500);
  await page.screenshot({
    path: "../dist/screenshots/rules.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Close detail" }).click();
  await page.getByRole("button", { name: "Assurance", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Assurance overview" }),
  ).toBeVisible();
  await page.screenshot({
    path: "../dist/screenshots/assurance.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
  expect(external).toEqual([]);
});
