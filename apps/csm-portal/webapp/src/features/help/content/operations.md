# Operations

Operations is mainly built for the SRE team, though any CS engineer can use
it. It's where the operational record types live: service requests, change
requests, incidents, and problems. These aren't limited to managed-cloud
work; they also cover other SaaS offerings. Each has its own tab in the
Operations sidebar section, its own list, and its own detail view: they
are separate record types, not case sub-types, even though a few of them link
back to a case.

## Service requests

The Service requests tab lists service-request cases using the same shared
issues list and filters as the Support section (case type is locked to
"Service request" here, so the type filter is hidden). Clicking a row opens
the same case detail view used everywhere else in the portal: overview,
comments, attachments, watchers, and the rest of the standard case tabs.

Use **Create service request** to open a form and raise a new one.

## Change requests

The Change requests tab lists change requests with server-side search,
pagination, and filters for state, impact, and closed-date range, plus a
CSV export of the filtered results. Each row links to a detail page.

The detail page shows:

- An **overview** card: project, type, linked case, deployment, deployed
  product, assigned engineer/team, duration, planned start/end, and audit
  fields.
- Tabs for **Approval**, **Plan**, **Comments**, and **Attachments**.
  - **Approval** shows the customer-approval/review flags plus a full
    approval-stage breakdown (e.g. Assess, Authorize, Customer Approval) with
    each individual approver's status. These flags are read-only in this
    portal — see below.
  - **Plan** shows the change-review packet: description, justification,
    impact description, rollback plan, test plan, service outage notes, and
    the communication plan.

From the detail page a CS engineer can:

- **Change state**: the action bar's buttons are driven entirely by the
  record's own legal next states, so only valid transitions are ever offered.
  Moving to a destructive state (rollback, cancel) requires typing a reason
  first, which is recorded as an internal note before the state change is
  applied.
- **Approve or reject** a pending approval stage, if the engineer is listed
  as an approver on it: the Approve/Reject buttons only appear on that
  engineer's own pending approval.
- **Edit** the planned window, assignment group, and rollback/test plans, or
  **Clone** the change request into a new one pre-filled with this one's
  values (useful for promoting the same change through another environment).
  The customer-approved/reviewed flags aren't editable here — they reflect an
  automation-only stage of the change's lifecycle and have no manual UI
  action in the backing system either.
- Add comments (public or internal) and upload/download attachments.

## Incidents

The Incidents tab lists incidents with server-side search, pagination, and
filters for priority, SLA-violated status, created-date range, and product,
plus a CSV export of the filtered results. Each row links to a detail page.

The detail page shows:

- An **overview** card: caller, assignment group, assigned to, opened date,
  created by, and last updated.
- Tabs for **Activities**, **Details**, **Related**, **Watchers**, and
  **Attachments**.
  - **Details** covers classification (category, subcategory, contact type,
    impact, urgency) and service/configuration-item information.
  - **Related** shows linked records (parent incident, change request,
    problem, and any linked service requests), plus a "caused by" reference
    shown as plain text since its target record type isn't confirmed.

From the detail page a CS engineer can:

- **Change state**: again driven by the incident's own legal next states.
  Moving to Resolved or Closed opens a dialog to collect a resolution code
  and notes, since those are required by the backing system for those two
  transitions.
- **Edit** the incident's fields.
- Manage the **watch list** (add or remove watchers).
- Add comments (public or internal) and upload/download attachments, with
  inline preview for supported attachment types.

## Problem management

The Problem management tab lists problems with server-side search,
pagination, free-text search, and a state filter. Each row links to a
detail page.

The detail page shows an overview (priority, category, subcategory, assigned
to, opened/closed dates), any linked records (origin record, primary
incident, linked change request, and linked incidents), and, once resolved,
a resolution section with resolution code, resolved-by, resolved-on, cause
notes, fix notes, and workaround.

Problem management is read-only in the portal today: there is no Edit dialog
and no state-changing action bar. Problems are owned by the SRE team, and the
portal doesn't yet expose a mutation endpoint for them the way it does for
change requests and incidents. A **Create problem** button on the list opens
a form to raise a new problem, but nothing on the detail page can be changed
from here afterward.
