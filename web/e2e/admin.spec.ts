import { test, expect } from "@playwright/test";
import ExcelJS from "exceljs";

test("admin setup, source previews, full-screen graph and separate approval", async ({
  page,
}) => {
  test.skip(
    !process.env.AUTODIT_TEST_PASSWORD,
    "Run through deployment-smoke.py --local --browser",
  );
  test.setTimeout(120000);
  page.setDefaultTimeout(10000);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  async function login(username: string, password: string) {
    await page.goto("/");
    await page.getByLabel("Username", { exact: true }).fill(username);
    await page.getByLabel("Password", { exact: true }).fill(password);
    await page.getByRole("button", { name: "Open workspace" }).click();
  }
  await login("admin", process.env.AUTODIT_TEST_PASSWORD!);
  await page
    .getByRole("button", { name: "Administration", exact: true })
    .click();
  await page.getByLabel("Username", { exact: true }).fill("approver");
  await page.getByLabel("Display name", { exact: true }).fill("Case Approver");
  await page
    .getByLabel("Initial password", { exact: false })
    .fill("Temporary approval password 42");
  await page.getByRole("checkbox", { name: /^Auditor / }).uncheck();
  await page.getByRole("checkbox", { name: /^Audit Manager / }).check();
  await page.getByRole("button", { name: "Save account", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Edit approver" }),
  ).toBeVisible();
  await page.getByRole("button", {name:"Workspace settings",exact:true}).click();
  await page.getByLabel("Workspace name").fill("Acceptance audit workspace");
  await page.getByLabel("Reporting currency", { exact: true }).fill("LKR");
  await page.getByLabel("Timezone", { exact: true }).fill("Asia/Colombo");
  await page.getByLabel("Date and number locale").fill("en-LK");
  await page.getByLabel("Fiscal year starts").selectOption("4");
  await page.getByRole("button", { name: "Save workspace settings" }).click();
  await expect(
    page.getByText("Workspace settings saved.", { exact: true }),
  ).toBeVisible();
  await page.getByLabel("Currency code", { exact: true }).fill("LKR");
  await page
    .getByLabel("Materiality threshold", { exact: true })
    .fill("1500.1234");
  await page.getByRole("button", { name: "Add currency", exact: true }).click();
  await page.getByRole("button", { name: "Release audit policy" }).click();
  await expect(
    page.getByText("Audit policy released for future runs.", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", {name:"Review workflow",exact:true}).click();
  await page.getByRole("button", { name: "Publish workflow" }).click();
  await expect(page.getByText(/Workflow published\./)).toBeVisible();
  await page.screenshot({
    path: "../dist/screenshots/admin.png",
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Sources & mapping", exact: true })
    .click();
  await page.getByLabel("Host", { exact: true }).fill("postgres");
  await page.getByLabel("Database", { exact: true }).fill("autodit");
  await page.getByLabel("Database username").fill("autodit_app");
  await page
    .getByLabel("Database password")
    .fill(process.env.AUTODIT_TEST_DB_PASSWORD!);
  await page
    .getByLabel("Allow an unencrypted connection on a trusted network")
    .check();
  await page
    .getByRole("button", { name: "Test connection", exact: true })
    .click();
  await expect(page.getByText(/Connection successful/)).toBeVisible();
  await page
    .getByRole("combobox", { name: "Table or view", exact: true })
    .selectOption({ label: "public.schema_version" });
  await page.getByRole("button", { name: "Preview table" }).click();
  await expect(
    page.getByRole("region", { name: "Record preview" }),
  ).toContainText("version");
  const workbook = new ExcelJS.Workbook();
  const sheet = workbook.addWorksheet("Payments");
  sheet.addRow(["id", "amount", "currency"]);
  sheet.addRow(["P1", "1200.1234", "LKR"]);
  await page
    .getByLabel("Choose extract file")
    .setInputFiles({
      name: "payments.xlsx",
      mimeType:
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      buffer: Buffer.from(await workbook.xlsx.writeBuffer()),
    });
  await page.getByLabel("Worksheet", { exact: false }).selectOption("Payments");
  await expect(
    page.getByRole("region", { name: "Record preview" }).first(),
  ).toContainText("1200.1234");
  await page.screenshot({
    path: "../dist/screenshots/source-previews.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "Rule catalog", exact: true }).click();
  await page.locator(".rule-card").first().click();
  await page.getByRole("button", { name: "Open decision graph" }).click();
  await expect(page.locator(".graph-editor")).toBeVisible();
  const box = await page.getByRole("dialog").boundingBox();
  expect(box?.width).toBeGreaterThan(1400);
  await page.locator(".graph-editor").scrollIntoViewIfNeeded();
  await page.screenshot({ path: "../dist/screenshots/fullscreen-graph.png" });
  await page.getByRole("button", { name: "Close detail" }).click();
  await page.getByRole("button", { name: "Review cases", exact: true }).click();
  await page
    .getByRole("button", { name: "Open case", exact: true })
    .first()
    .click();
  await page
    .getByLabel("Review note or reason")
    .fill("Reviewed source evidence and control totals.");
  await page
    .getByRole("button", { name: "Complete step", exact: true })
    .click();
  await expect(
    page.getByText(/A different person must complete/),
  ).toBeVisible();
  await page.getByLabel("Closure outcome").selectOption("accepted");
  await page
    .getByLabel("Review note or reason")
    .fill("Attempt same-user approval.");
  await expect(
    page.getByRole("button", { name: "Approve and close case" }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Close detail" }).click();
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(page.getByRole("heading", {name:"Sign in to your workspace"})).toBeVisible();
  await login("approver", "Temporary approval password 42");
  await expect(
    page.getByRole("heading", { name: "Choose your own password" }),
  ).toBeVisible();
  await page
    .getByLabel("Current password", { exact: true })
    .fill("Temporary approval password 42");
  await page
    .getByLabel("New password", { exact: true })
    .fill("Replacement approval password 73");
  await page
    .getByLabel("Confirm new password", { exact: true })
    .fill("Replacement approval password 73");
  await page
    .getByRole("button", { name: "Save password and sign in again" })
    .click();
  await expect(page.getByRole("heading", {name:"Sign in to your workspace"})).toBeVisible();
  await login("approver", "Replacement approval password 73");
  await expect(
    page.getByRole("button", { name: "Administration", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Review cases", exact: true }).click();
  await page
    .locator("tbody tr")
    .filter({ hasText: "Approve and close" })
    .getByRole("button", { name: "Open case" })
    .click();
  await page
    .getByLabel("Review note or reason")
    .fill("Independent approval completed with supporting evidence.");
  await page.getByLabel("Closure outcome").selectOption("accepted");
  await page.getByRole("button", { name: "Approve and close case" }).click();
  await page.getByLabel("Show cases").selectOption("closed");
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await page.getByRole("button", { name: "Open case" }).click();
  await expect(
    page.getByText("Independent approval completed with supporting evidence."),
  ).toBeVisible();
  await page.screenshot({ path: "../dist/screenshots/case-approval.png" });
  expect(errors).toEqual([]);
});
