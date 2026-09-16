import { Help } from "./Shared";
const guides: Record<string, string[]> = {
  queue: [
    "Filter by status, rule or priority to find work that needs attention.",
    "Open a finding to inspect the evidence and verify its replay. Add a note describing your assessment.",
    "If the finding has a review case, complete its configured review and approval steps in Review cases.",
  ],
  runs: [
    "Open a run to see extraction, validation, reconciliation and scoring progress.",
    "If a run stops, inspect the stage and control differences, correct the source extract, and submit the full population again.",
    "An empty findings queue does not prove a run completed. Check its completion status here.",
  ],
  rules: [
    "Open a rule to edit severity and reviewer routing. The editor fills the browser window.",
    "Technical rule authors can open the decision graph and edit its decision table.",
    "Simulate the draft before releasing it. Ask an administrator to configure currencies and other policy inputs.",
  ],
  sources: [
    "Register a source, then test a database connection or upload a file to preview its records.",
    "Map the columns and attach independent control totals before starting an audit run.",
    "Technical connector access is separate from audit review and approval permissions.",
  ],
  cases: [
    "Open a case to see the steps required before closure.",
    "Review the evidence, record a note and complete your assigned step. Send it back when further work is needed.",
    "Approvals marked for a different person cannot be completed by the previous reviewer.",
  ],
  admin: [
    "Create named accounts and select the roles that match each person's responsibilities.",
    "Configure the workspace name, currency policies, timezone, locale and fiscal year.",
    "Publish the review workflow after creating accounts for its reviewers and approvers.",
  ],
  assurance: [
    "Compare tested populations, queue status and data freshness.",
    "Use these figures with the run monitor. Untested controls are not treated as passed.",
    "Open the exception queue to investigate the findings behind the totals.",
  ],
};
export function PageGuide({ page }: { page: string }) {
  return (
    <Help title="Page guide">
      <ol>
        {guides[page]?.map((text) => (
          <li key={text}>{text}</li>
        ))}
      </ol>
    </Help>
  );
}
