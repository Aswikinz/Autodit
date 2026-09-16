import { test, expect, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import ExcelJS from "exceljs";

const invoiceCSV = [
  "invoice id,amount,department,ignored",
  "INV1,500,Travel,x",
  "INV2,1500,Travel,y",
  "INV3,2000,Sales,z",
].join("\n");

async function signIn(
  page: Page,
  username = "admin",
  password = process.env.AUTODIT_TEST_PASSWORD!,
) {
  await page.goto("/");
  await page.getByLabel("Username", { exact: true }).fill(username);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Open workspace" }).click();
  await page.getByRole("button", { name: "Analyze data", exact: true }).click();
}

async function uploadCSV(page: Page, csv = invoiceCSV) {
  await page.getByLabel("Choose data file").setInputFiles({
    name: "invoices.csv",
    mimeType: "text/csv",
    buffer: Buffer.from(csv),
  });
}

async function addCondition(
  page: Page,
  column: string,
  condition: string,
  value: string,
) {
  await page
    .getByRole("combobox", { name: "Column", exact: true })
    .selectOption({ label: column });
  await page
    .getByRole("combobox", { name: "Condition", exact: true })
    .selectOption({ label: condition });
  await page.getByLabel("Value", { exact: true }).fill(value);
  await page
    .getByRole("button", { name: "Add condition", exact: true })
    .click();
}

test.beforeEach(async ({ page }) => {
  test.skip(
    !process.env.AUTODIT_TEST_PASSWORD,
    "Run through deployment-smoke.py --local --browser",
  );
  page.setDefaultTimeout(15000);
  await page.addInitScript(() => {
    const state = window as typeof window & { __autoditWasmReady: number };
    state.__autoditWasmReady = 0;
    for (const method of ["instantiate", "instantiateStreaming"] as const) {
      const original = WebAssembly[method];
      Object.defineProperty(WebAssembly, method, {
        configurable: true,
        writable: true,
        value: async (...args: unknown[]) => {
          const result = await Reflect.apply(original, WebAssembly, args);
          state.__autoditWasmReady += 1;
          return result;
        },
      });
    }
  });
  await signIn(page);
});

test("CSV columns, combined conditions, full-screen graph and saved results survive reload", async ({
  page,
}) => {
  test.setTimeout(120000);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));

  await page.screenshot({
    path: "../dist/screenshots/analysis-load.png",
    fullPage: true,
  });
  await uploadCSV(page);
  await expect(
    page.getByRole("heading", { name: "Choose columns" }),
  ).toBeVisible();
  await expect(
    page.getByRole("region", { name: "Data preview" }),
  ).toContainText("INV2");
  await page.getByLabel("Use ignored", { exact: true }).uncheck();
  await page.getByLabel("amount type", { exact: true }).selectOption("number");
  await page.screenshot({
    path: "../dist/screenshots/analysis-columns.png",
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Continue to rules", exact: true })
    .click();
  await addCondition(page, "amount", "Greater than", "1000");
  await addCondition(page, "department", "Equals", "Travel");
  const name = `Travel threshold ${Date.now()}`;
  await page.getByLabel("Analysis name", { exact: true }).fill(name);
  await page.screenshot({
    path: "../dist/screenshots/analysis-rules.png",
    fullPage: true,
  });

  await page
    .getByRole("button", { name: "Open decision graph", exact: true })
    .click();
  const graph = page.getByRole("dialog", { name: "Decision graph" });
  await expect(graph.locator(".graph-editor")).toBeVisible();
  await expect(graph.locator(".graph-editor")).toHaveAttribute(
    "data-editor-ready",
    "true",
  );
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as typeof window & { __autoditWasmReady: number })
            .__autoditWasmReady,
      ),
    )
    .toBeGreaterThan(0);
  const graphBounds = await graph.boundingBox();
  expect(graphBounds?.width).toBeGreaterThan(1400);
  expect(graphBounds?.height).toBeGreaterThan(850);
  await page.screenshot({ path: "../dist/screenshots/analysis-graph.png" });
  const ruleNode = graph.locator('.react-flow__node[data-id="autodit-rules"]');
  await expect(ruleNode).toBeVisible();
  const beforeDrag = await ruleNode.boundingBox();
  const handle = await ruleNode.locator(".grl-dn__header__icon").boundingBox();
  expect(beforeDrag).not.toBeNull();
  expect(handle).not.toBeNull();
  await page.mouse.move(
    handle!.x + handle!.width / 2,
    handle!.y + handle!.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    handle!.x + handle!.width / 2 + 80,
    handle!.y + handle!.height / 2 + 40,
    { steps: 8 },
  );
  await page.mouse.up();
  await expect
    .poll(async () =>
      Math.abs((await ruleNode.boundingBox())!.x - beforeDrag!.x),
    )
    .toBeGreaterThan(40);
  await ruleNode
    .getByRole("button", { name: "Edit Table", exact: true })
    .click();
  await expect(graph).toContainText("department");
  await expect(graph).toContainText("amount");
  await page.getByRole("button", { name: "Close detail", exact: true }).click();
  await page.getByRole("button", { name: "Test rules", exact: true }).click();

  await expect(
    page.getByRole("heading", { name: "Results", exact: true }),
  ).toBeVisible();
  const results = page.getByRole("region", { name: "Analysis results" });
  await expect(results.locator("tbody tr")).toHaveCount(3);
  await expect(
    results.getByRole("columnheader", { name: "ignored", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: /^Flagged/ }).click();
  await expect(results.locator("tbody tr")).toHaveCount(1);
  await expect(results).toContainText("INV2");
  await expect(results).not.toContainText("INV1");
  await expect(results).not.toContainText("INV3");
  await page
    .getByRole("button", { name: "Inspect row 2", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Decision graph" }),
  ).toContainText("Row 2: Flagged");
  await page.getByRole("button", { name: "Close detail", exact: true }).click();
  await page.screenshot({
    path: "../dist/screenshots/analysis-results.png",
    fullPage: true,
  });

  const downloaded = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Export results", exact: true })
    .click();
  const download = await downloaded;
  expect(download.suggestedFilename()).toMatch(/\.csv$/);
  const exportText = await readFile((await download.path())!, "utf8");
  expect(exportText).toContain("INV2");
  expect(exportText).not.toContain("ignored");
  expect(exportText).not.toContain("INV1");
  expect(exportText).not.toContain("INV3");

  await page
    .getByRole("button", { name: "Save analysis", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText(/saved/i);
  await page.reload();
  await page.getByRole("button", { name: new RegExp(name) }).click();
  await page.getByRole("button", { name: "Test rules", exact: true }).click();
  await page.getByRole("button", { name: /^Flagged/ }).click();
  await expect(
    page.getByRole("region", { name: "Analysis results" }).locator("tbody tr"),
  ).toHaveCount(1);
  await expect(
    page.getByRole("region", { name: "Analysis results" }),
  ).toContainText("INV2");
  expect(errors).toEqual([]);
});

test("Excel worksheet selection feeds the same rule flow", async ({ page }) => {
  const workbook = new ExcelJS.Workbook();
  const summary = workbook.addWorksheet("Summary");
  summary.addRows([["description"], ["Not the transaction worksheet"]]);
  const invoices = workbook.addWorksheet("Invoices");
  invoices.addRows([
    ["invoice id", "amount"],
    ["XLSX1", 1999.125],
    ["XLSX2", 499.75],
  ]);
  await page.getByLabel("Choose data file").setInputFiles({
    name: "invoices.xlsx",
    mimeType:
      "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    buffer: Buffer.from(await workbook.xlsx.writeBuffer()),
  });
  await page.getByLabel("Worksheet", { exact: false }).selectOption("Invoices");
  await page
    .getByRole("button", { name: "Preview worksheet", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Data preview" }),
  ).toContainText("1999.125");
  await expect(
    page.getByRole("region", { name: "Data preview" }),
  ).not.toContainText("Not the transaction worksheet");
  await page.getByLabel("amount type", { exact: true }).selectOption("number");
  await page
    .getByRole("button", { name: "Continue to rules", exact: true })
    .click();
  await addCondition(page, "amount", "Greater than", "1000");
  await page.getByRole("button", { name: "Test rules", exact: true }).click();
  await page.getByRole("button", { name: /^Flagged/ }).click();
  const results = page.getByRole("region", { name: "Analysis results" });
  await expect(results.locator("tbody tr")).toHaveCount(1);
  await expect(results).toContainText("XLSX1");
});

test("changing selected columns invalidates results and removes obsolete conditions", async ({
  page,
}) => {
  await uploadCSV(page);
  await page
    .getByRole("button", { name: "Continue to rules", exact: true })
    .click();
  await addCondition(page, "amount", "Greater than", "1000");
  await page.getByRole("button", { name: "Test rules", exact: true }).click();
  await page.getByRole("button", { name: "Flagged", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Analysis results" }).locator("tbody tr"),
  ).toHaveCount(2);
  await page.getByRole("button", { name: "Edit rules", exact: true }).click();
  const steps = page.getByRole("navigation", { name: "Analysis steps" });
  await steps
    .getByRole("button", { name: "Choose columns", exact: true })
    .click();
  await page.getByLabel("Use amount", { exact: true }).uncheck();
  await expect(steps.getByRole("button", { name: /Results/ })).toBeDisabled();
  await steps.getByRole("button", { name: /Configure rules/ }).click();
  await expect(
    page.getByRole("button", { name: "Remove condition 1", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Test rules", exact: true }).click();
  const results = page.getByRole("region", { name: "Analysis results" });
  await expect(
    results.locator("tbody tr").filter({ hasText: "Passed" }),
  ).toHaveCount(3);
  await expect(
    results.getByRole("columnheader", { name: "amount", exact: true }),
  ).toHaveCount(0);
});

test("JSON rows support text conditions without a financial mapping contract", async ({
  page,
}) => {
  await page.getByLabel("Choose data file").setInputFiles({
    name: "access.json",
    mimeType: "application/json",
    buffer: Buffer.from(
      JSON.stringify([
        { employee: "Ada", access: "privileged" },
        { employee: "Grace", access: "standard" },
      ]),
    ),
  });
  await expect(
    page.getByRole("region", { name: "Data preview" }),
  ).toContainText("Grace");
  await page
    .getByRole("button", { name: "Continue to rules", exact: true })
    .click();
  await addCondition(page, "access", "Equals", "privileged");
  await page.getByRole("button", { name: "Test rules", exact: true }).click();
  await page.getByRole("button", { name: /^Flagged/ }).click();
  const results = page.getByRole("region", { name: "Analysis results" });
  await expect(results.locator("tbody tr")).toHaveCount(1);
  await expect(results).toContainText("Ada");
  await expect(results).not.toContainText("Grace");
});

test("a database connection and selected table feed the rule workflow", async ({
  page,
}) => {
  test.skip(
    !process.env.AUTODIT_TEST_DB_PASSWORD,
    "Requires the isolated smoke database.",
  );
  await page.getByRole("button", { name: "Database", exact: true }).click();
  await page.getByLabel("Host", { exact: true }).fill("postgres");
  await page.getByLabel("Database", { exact: true }).fill("autodit");
  await page
    .getByLabel("Database username", { exact: true })
    .fill("autodit_app");
  await page
    .getByLabel("Database password", { exact: true })
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
  await page.getByRole("button", { name: "Load table", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Choose columns" }),
  ).toBeVisible();
  const rows = await page
    .getByRole("region", { name: "Data preview" })
    .locator("tbody tr")
    .count();
  expect(rows).toBeGreaterThanOrEqual(7);
  await page.getByLabel("version type", { exact: true }).selectOption("number");
  await page
    .getByRole("button", { name: "Continue to rules", exact: true })
    .click();
  await addCondition(page, "version", "Greater than", "0");
  await page.getByRole("button", { name: "Test rules", exact: true }).click();
  await page.getByRole("button", { name: "Flagged", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Analysis results" }).locator("tbody tr"),
  ).toHaveCount(rows);
});

test("the Function editor loads locally and executes an edited graph", async ({
  page,
}) => {
  test.setTimeout(120000);
  const origin = new URL(page.url()).origin;
  const external: string[] = [];
  page.on("request", (request) => {
    if (
      /^https?:/.test(request.url()) &&
      new URL(request.url()).origin !== origin
    )
      external.push(request.url());
  });
  await page.route(/^https:\/\//, (route) => route.abort());
  const name = `Function editor ${Date.now()}`;
  const columns = [{ name: "amount", type: "number" }];
  const source =
    "export const handler = async input => ({ flag: input.data.amount > 1000 });";
  const model = {
    nodes: [
      {
        id: "request",
        name: "Your data",
        type: "inputNode",
        position: { x: 80, y: 180 },
        content: { schema: "" },
      },
      {
        id: "function",
        name: "Amount check",
        type: "functionNode",
        position: { x: 380, y: 180 },
        content: { source },
      },
      {
        id: "response",
        name: "Results",
        type: "outputNode",
        position: { x: 710, y: 180 },
        content: { schema: "" },
      },
    ],
    edges: [
      { id: "request-function", sourceId: "request", targetId: "function" },
      { id: "function-response", sourceId: "function", targetId: "response" },
    ],
  };
  const session = await (await page.request.get("/api/session")).json();
  const save = await page.request.post("/api/analyses", {
    headers: { Origin: origin, "X-CSRF-Token": session.identity.csrf_token },
    data: {
      name,
      revision: 0,
      dataset: {
        columns,
        rows: [{ amount: "500" }, { amount: "1500" }],
        row_count: 2,
      },
      selected_columns: columns,
      model,
    },
  });
  expect(save.status()).toBe(200);
  await page.reload();
  await page.getByRole("button", { name: new RegExp(name) }).click();
  await page
    .getByRole("button", { name: "Open decision graph", exact: true })
    .click();
  const graph = page.getByRole("dialog", { name: "Decision graph" });
  await graph
    .getByRole("button", { name: "Edit Function", exact: true })
    .click();
  const editor = graph.locator(".monaco-editor textarea.inputarea").first();
  await expect(editor).toBeVisible({ timeout: 30000 });
  await editor.focus();
  await page.keyboard.press("Control+a");
  await page.keyboard.insertText(
    "export const handler = async input => ({ flag: input.data.amount > 100000 });",
  );
  await expect(
    graph.getByRole("button", { name: "Save analysis", exact: true }),
  ).toBeEnabled();
  await page.screenshot({ path: "../dist/screenshots/analysis-function.png" });
  await page.getByRole("button", { name: "Close detail", exact: true }).click();
  await page.getByRole("button", { name: "Test rules", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Results", exact: true }),
  ).toBeVisible();
  const results = page.getByRole("region", { name: "Analysis results" });
  await expect(results.locator("tbody tr")).toHaveCount(2);
  await expect(
    results.locator("tbody tr").filter({ hasText: "Passed" }),
  ).toHaveCount(2);
  await page.getByRole("button", { name: "Flagged", exact: true }).click();
  await expect(results.locator("tbody tr")).toHaveCount(0);
  expect(external).toEqual([]);
});

test("invalid upload and invalid typed values are visible and recoverable", async ({
  page,
}) => {
  await uploadCSV(page, "id,id\nA,B\n");
  await expect(page.getByRole("alert")).toBeVisible();
  await uploadCSV(page, "invoice id,amount\nBAD,not-a-number\nGOOD,1200\n");
  await expect(
    page.getByRole("heading", { name: "Choose columns" }),
  ).toBeVisible();
  await page.getByLabel("amount type", { exact: true }).selectOption("number");
  await page
    .getByRole("button", { name: "Continue to rules", exact: true })
    .click();
  await addCondition(page, "amount", "Greater than", "1000");
  await page.getByRole("button", { name: "Test rules", exact: true }).click();
  await page.getByRole("button", { name: /^Errors/ }).click();
  const results = page.getByRole("region", { name: "Analysis results" });
  await expect(results.locator("tbody tr")).toHaveCount(1);
  await expect(results).toContainText("BAD");
  await page.getByRole("button", { name: /^Flagged/ }).click();
  await expect(results.locator("tbody tr")).toHaveCount(1);
  await expect(results).toContainText("GOOD");
  await expect(results).not.toContainText("BAD");
});

test("an auditor can test data but cannot save an analysis through the UI or API", async ({
  page,
}) => {
  test.setTimeout(120000);
  const username = `analysis-reviewer-${Date.now()}`;
  const temporaryPassword = "Temporary analysis review 42";
  const finalPassword = "Replacement analysis review 73";
  const origin = new URL(page.url()).origin;
  const adminSession = await (await page.request.get("/api/session")).json();
  const create = await page.request.post("/api/admin/users", {
    headers: {
      Origin: origin,
      "X-CSRF-Token": adminSession.identity.csrf_token,
    },
    data: {
      username,
      display_name: "Analysis reviewer",
      roles: ["auditor"],
      enabled: true,
      revision: 0,
      password: temporaryPassword,
    },
  });
  expect(create.status()).toBe(204);
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await page.getByLabel("Username", { exact: true }).fill(username);
  await page.getByLabel("Password", { exact: true }).fill(temporaryPassword);
  await page.getByRole("button", { name: "Open workspace" }).click();
  await expect(
    page.getByRole("heading", { name: "Choose your own password" }),
  ).toBeVisible();
  await page
    .getByLabel("Current password", { exact: true })
    .fill(temporaryPassword);
  await page.getByLabel("New password", { exact: true }).fill(finalPassword);
  await page
    .getByLabel("Confirm new password", { exact: true })
    .fill(finalPassword);
  await page
    .getByRole("button", { name: "Save password and sign in again" })
    .click();
  await expect(
    page.getByRole("heading", { name: "Sign in to your workspace" }),
  ).toBeVisible();
  await signIn(page, username, finalPassword);
  await expect(
    page.getByRole("button", { name: "Administration", exact: true }),
  ).toHaveCount(0);

  await uploadCSV(page);
  await page
    .getByRole("button", { name: "Continue to rules", exact: true })
    .click();
  await addCondition(page, "amount", "Greater than", "1000");
  await expect(
    page.getByRole("button", { name: "Save analysis", exact: true }),
  ).toHaveCount(0);
  const sent = page.waitForRequest(
    (request) =>
      request.url().endsWith("/api/analyses/test") &&
      request.method() === "POST",
  );
  await page.getByRole("button", { name: "Test rules", exact: true }).click();
  const payload = (await sent).postDataJSON();
  await expect(
    page.getByRole("heading", { name: "Results", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Flagged", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Analysis results" }).locator("tbody tr"),
  ).toHaveCount(2);
  await expect(
    page.getByRole("button", { name: "Save analysis", exact: true }),
  ).toHaveCount(0);
  const reviewerSession = await (await page.request.get("/api/session")).json();
  const forbidden = await page.request.post("/api/analyses", {
    headers: {
      Origin: origin,
      "X-CSRF-Token": reviewerSession.identity.csrf_token,
    },
    data: { ...payload, name: "Unauthorized saved analysis", revision: 0 },
  });
  expect(forbidden.status()).toBe(403);
});
