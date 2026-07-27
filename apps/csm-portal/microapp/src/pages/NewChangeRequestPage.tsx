// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import { useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  AdapterDateFns,
  Button,
  DatePickers,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown } from "@wso2/oxygen-ui-icons-react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { changeRequests } from "@src/services/changeRequests";
import { changeRequestLookups, type EntityOption } from "@src/services/changeRequestLookups";
import { users } from "@src/services/users";
import type {
  ChangeRequestCategory,
  ChangeRequestCreatePayloadDto,
  ChangeRequestPriority,
  ChangeRequestRisk,
  ChangeRequestState,
  ChangeRequestType,
} from "@src/types";
import { Logger } from "@utils/logger";
import { changeRequestStateLabel } from "@components/operations/config";
import { AsyncEntitySelect } from "@components/operations/AsyncEntitySelect";

const { DateTimePicker, LocalizationProvider } = DatePickers;

const UNSET = "";
const SELECT_PLACEHOLDER = "-- Select --";
const SUBJECT_MAX = 500;

const TYPE_OPTIONS: Array<{ value: ChangeRequestType; label: string }> = [
  { value: "standard", label: "Standard" },
  { value: "normal", label: "Normal" },
  { value: "emergency", label: "Emergency" },
  { value: "model", label: "Model" },
  { value: "site_reliability_ops", label: "Site reliability ops" },
  { value: "azure", label: "Azure" },
];

const CATEGORY_OPTIONS: Array<{ value: ChangeRequestCategory; label: string }> = [
  { value: "hardware", label: "Hardware" },
  { value: "software", label: "Software" },
  { value: "service", label: "Service" },
  { value: "system_software", label: "System software" },
  { value: "applications_software", label: "Applications software" },
  { value: "network", label: "Network" },
  { value: "telecom", label: "Telecom" },
  { value: "documentation", label: "Documentation" },
  { value: "regular_release_cloud", label: "Regular release (cloud)" },
  { value: "hotfix_release_cloud", label: "Hotfix release (cloud)" },
  { value: "devops", label: "DevOps" },
  { value: "cloud_computing", label: "Cloud computing" },
  { value: "other", label: "Other" },
];

const IMPACT_OPTIONS: Array<{ value: "high" | "medium" | "low"; label: string }> = [
  { value: "high", label: "High" },
  { value: "medium", label: "Medium" },
  { value: "low", label: "Low" },
];

const PRIORITY_OPTIONS: Array<{ value: ChangeRequestPriority; label: string }> = [
  { value: "critical", label: "Critical" },
  { value: "high", label: "High" },
  { value: "moderate", label: "Moderate" },
  { value: "low", label: "Low" },
];

const RISK_OPTIONS: Array<{ value: ChangeRequestRisk; label: string }> = [
  { value: "high", label: "High" },
  { value: "moderate", label: "Moderate" },
  { value: "low", label: "Low" },
];

// Only the pre-workflow states are selectable at creation — a new change request must enter its
// lifecycle at the start (new/assess/authorize) and move forward from there, same restriction the
// webapp's create form applies.
const CREATE_STATE_VALUES: ChangeRequestState[] = ["new", "assess", "authorize"];
const STATE_OPTIONS: Array<{ value: ChangeRequestState; label: string }> = CREATE_STATE_VALUES.map((s) => ({
  value: s,
  label: changeRequestStateLabel(s),
}));

/** "Characters left: N" once a field is more than half full, matching the webapp's live counter. */
function charsLeftHelper(value: string, max: number): string | undefined {
  return value.length >= max / 2 ? `Characters left: ${max - value.length}` : undefined;
}

/** Local Date to the backend's expected "YYYY-MM-DD HH:MM:SS" string. */
function toBackendDateTime(date: Date): string {
  const y = date.getFullYear();
  const mo = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  const h = String(date.getHours()).padStart(2, "0");
  const mi = String(date.getMinutes()).padStart(2, "0");
  const s = String(date.getSeconds()).padStart(2, "0");
  return `${y}-${mo}-${d} ${h}:${mi}:${s}`;
}

// Ports the webapp's CreateChangeRequestPage.tsx. Two sections, same as the webapp: the core
// fields (subject, type/category/priority/impact/risk/state, six Planning fields, planned
// start/end) always visible, and a collapsed "More options" section for the six ServiceNow
// reference lookups (Service/Service offering/Configuration item/Assignment group/Assigned
// to/Requested by) plus the two journal fields — collapsed by default since they're used less
// often at creation time, same as the webapp's own Accordion. Layout follows this app's own
// NewCasePage.tsx/NewServiceRequestPage.tsx (single-column Stack) rather than the webapp's
// Grid/Card, and the six Planning fields are plain multiline TextFields rather than the webapp's
// rich-text editor (this app has no rich-text component) — same deviation those two pages made.
export default function NewChangeRequestPage() {
  const navigate = useNavigate();

  const [subject, setSubject] = useState("");
  // Pre-selected to match the webapp's own defaults (itself matching the legacy ServiceNow form):
  // most change requests are Normal type, Other category, Low impact, Moderate risk. Priority has
  // no default there either, so it stays unset here too.
  const [type, setType] = useState<ChangeRequestType | typeof UNSET>("normal");
  const [category, setCategory] = useState<ChangeRequestCategory | typeof UNSET>("other");
  const [impact, setImpact] = useState<"high" | "medium" | "low" | typeof UNSET>("low");
  const [priority, setPriority] = useState<ChangeRequestPriority | typeof UNSET>(UNSET);
  const [risk, setRisk] = useState<ChangeRequestRisk | typeof UNSET>("moderate");
  const [state, setState] = useState<ChangeRequestState | typeof UNSET>("new");
  const [plannedStartDate, setPlannedStartDate] = useState<Date | null>(null);
  const [plannedEndDate, setPlannedEndDate] = useState<Date | null>(null);
  const [description, setDescription] = useState("");
  const [justification, setJustification] = useState("");
  const [implementationPlan, setImplementationPlan] = useState("");
  const [riskImpactAnalysis, setRiskImpactAnalysis] = useState("");
  const [backoutPlan, setBackoutPlan] = useState("");
  const [testPlan, setTestPlan] = useState("");
  const [service, setService] = useState<EntityOption | null>(null);
  const [serviceOffering, setServiceOffering] = useState<EntityOption | null>(null);
  const [configurationItem, setConfigurationItem] = useState<EntityOption | null>(null);
  const [group, setGroup] = useState<EntityOption | null>(null);
  const [assignedEngineer, setAssignedEngineer] = useState<EntityOption | null>(null);
  const [requestedBy, setRequestedBy] = useState<EntityOption | null>(null);
  const [comment, setComment] = useState("");
  const [workNote, setWorkNote] = useState("");
  const [submitError, setSubmitError] = useState<string | null>(null);

  // Defaults "Requested by" to the signed-in user, matching the webapp's own create form (the
  // backend doesn't do this itself). Fires once, when the current user first loads; a ref (not
  // the field's own emptiness) gates it so manually clearing the field afterward sticks. Adjusted
  // during render (React's recommended pattern for this) rather than in an effect, which would
  // call setState synchronously post-commit — same approach the webapp's own page takes.
  const { data: me } = useQuery(users.me());
  const autoFilledRequester = useRef(false);
  if (me?.id && !autoFilledRequester.current) {
    autoFilledRequester.current = true;
    setRequestedBy({ id: me.id, label: me.fullName });
  }

  const createChangeRequest = useMutation({ mutationFn: changeRequests.create });

  const canSubmit = subject.trim().length > 0 && !createChangeRequest.isPending;

  const handleSubmit = (): void => {
    if (!canSubmit) return;
    setSubmitError(null);

    const payload: ChangeRequestCreatePayloadDto = { subject: subject.trim() };
    if (type) payload.type = type;
    if (category) payload.category = category;
    if (impact) payload.impact = impact;
    if (priority) payload.priority = priority;
    if (risk) payload.risk = risk;
    if (state) payload.state = state;
    if (plannedStartDate) payload.plannedStartDate = toBackendDateTime(plannedStartDate);
    if (plannedEndDate) payload.plannedEndDate = toBackendDateTime(plannedEndDate);
    if (description.trim()) payload.description = description.trim();
    if (justification.trim()) payload.justification = justification.trim();
    if (implementationPlan.trim()) payload.implementationPlan = implementationPlan.trim();
    if (riskImpactAnalysis.trim()) payload.riskImpactAnalysis = riskImpactAnalysis.trim();
    if (backoutPlan.trim()) payload.backoutPlan = backoutPlan.trim();
    if (testPlan.trim()) payload.testPlan = testPlan.trim();
    if (service) payload.serviceId = service.id;
    if (serviceOffering) payload.serviceOfferingId = serviceOffering.id;
    if (configurationItem) payload.configurationItemId = configurationItem.id;
    if (group) payload.groupId = group.id;
    if (assignedEngineer) payload.assignedEngineerId = assignedEngineer.id;
    if (requestedBy) payload.requestedById = requestedBy.id;
    if (comment.trim()) payload.comment = comment.trim();
    if (workNote.trim()) payload.workNote = workNote.trim();

    createChangeRequest.mutate(payload, {
      onSuccess: (created) => navigate(`/operations/change-requests/${created.changeRequest.id}`),
      onError: (err) => {
        Logger.warn("Could not create the change request", err);
        setSubmitError("Could not create the change request. Please try again.");
      },
    });
  };

  // Shared renderer for a "-- Select --" dropdown — every field here is optional, so clearing back
  // to the placeholder is valid and just omits the field from the payload.
  const renderSelect = <T extends string>(
    id: string,
    label: string,
    value: T | typeof UNSET,
    onChange: (v: T | typeof UNSET) => void,
    options: Array<{ value: T; label: string }>,
  ) => (
    <FormControl fullWidth size="small" disabled={createChangeRequest.isPending}>
      <InputLabel id={`${id}-label`} shrink>
        {label}
      </InputLabel>
      <Select
        labelId={`${id}-label`}
        label={label}
        value={value}
        displayEmpty
        onChange={(e) => onChange(e.target.value as T | typeof UNSET)}
      >
        <MenuItem value={UNSET}>
          <Typography component="span" color="text.secondary">
            {SELECT_PLACEHOLDER}
          </Typography>
        </MenuItem>
        {options.map((o) => (
          <MenuItem key={o.value} value={o.value}>
            {o.label}
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  );

  return (
    <Stack gap={2}>
      <Typography variant="h6">New Change Request</Typography>

      <Stack gap={2}>
        <TextField
          label="Subject"
          value={subject}
          onChange={(e) => setSubject(e.target.value.slice(0, SUBJECT_MAX))}
          size="small"
          fullWidth
          required
          disabled={createChangeRequest.isPending}
          placeholder="Short summary of the change"
          helperText={charsLeftHelper(subject, SUBJECT_MAX)}
        />

        {renderSelect("cr-type", "Type", type, setType, TYPE_OPTIONS)}
        {renderSelect("cr-category", "Category", category, setCategory, CATEGORY_OPTIONS)}
        {renderSelect("cr-priority", "Priority", priority, setPriority, PRIORITY_OPTIONS)}
        {renderSelect("cr-impact", "Impact", impact, setImpact, IMPACT_OPTIONS)}
        {renderSelect("cr-risk", "Risk", risk, setRisk, RISK_OPTIONS)}
        {renderSelect("cr-state", "State", state, setState, STATE_OPTIONS)}

        <Typography variant="subtitle2">Planning</Typography>

        <TextField
          label="Description"
          placeholder="What is changing?"
          size="small"
          fullWidth
          multiline
          minRows={3}
          disabled={createChangeRequest.isPending}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
        <TextField
          label="Justification"
          placeholder="Why is this change needed?"
          size="small"
          fullWidth
          multiline
          minRows={3}
          disabled={createChangeRequest.isPending}
          value={justification}
          onChange={(e) => setJustification(e.target.value)}
        />
        <TextField
          label="Implementation plan"
          size="small"
          fullWidth
          multiline
          minRows={3}
          disabled={createChangeRequest.isPending}
          value={implementationPlan}
          onChange={(e) => setImplementationPlan(e.target.value)}
        />
        <TextField
          label="Risk and impact analysis"
          size="small"
          fullWidth
          multiline
          minRows={3}
          disabled={createChangeRequest.isPending}
          value={riskImpactAnalysis}
          onChange={(e) => setRiskImpactAnalysis(e.target.value)}
        />
        <TextField
          label="Backout plan"
          size="small"
          fullWidth
          multiline
          minRows={3}
          disabled={createChangeRequest.isPending}
          value={backoutPlan}
          onChange={(e) => setBackoutPlan(e.target.value)}
        />
        <TextField
          label="Test plan"
          size="small"
          fullWidth
          multiline
          minRows={3}
          disabled={createChangeRequest.isPending}
          value={testPlan}
          onChange={(e) => setTestPlan(e.target.value)}
        />

        <Typography variant="subtitle2">Schedule</Typography>

        <LocalizationProvider dateAdapter={AdapterDateFns}>
          <DateTimePicker
            label="Planned start"
            value={plannedStartDate}
            maxDateTime={plannedEndDate ?? undefined}
            onChange={(date) =>
              setPlannedStartDate(date instanceof Date && !Number.isNaN(date.getTime()) ? date : null)
            }
            disabled={createChangeRequest.isPending}
            slotProps={{ textField: { size: "small", fullWidth: true }, field: { clearable: true } }}
          />
          <DateTimePicker
            label="Planned end"
            value={plannedEndDate}
            minDateTime={plannedStartDate ?? undefined}
            onChange={(date) => setPlannedEndDate(date instanceof Date && !Number.isNaN(date.getTime()) ? date : null)}
            disabled={createChangeRequest.isPending}
            slotProps={{ textField: { size: "small", fullWidth: true }, field: { clearable: true } }}
          />
        </LocalizationProvider>

        {/* Everything below is optional and used less often at creation time — collapsed by
            default so the form isn't dominated by fields most requests won't need up front. */}
        <Accordion disableGutters sx={{ "&:before": { display: "none" } }}>
          <AccordionSummary expandIcon={<ChevronDown size={16} />}>
            <Typography variant="body2" color="text.secondary">
              More options (optional)
            </Typography>
          </AccordionSummary>
          <AccordionDetails sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
            {/* These map to two different ServiceNow journal fields — mixing them up would either
                leak an internal note to the customer or bury something they were meant to see. */}
            <TextField
              label="Additional comments (customer-visible)"
              multiline
              minRows={2}
              size="small"
              fullWidth
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              disabled={createChangeRequest.isPending}
              helperText="Visible to the customer — do not include internal-only details."
            />
            <TextField
              label="Internal work note"
              multiline
              minRows={2}
              size="small"
              fullWidth
              value={workNote}
              onChange={(e) => setWorkNote(e.target.value)}
              disabled={createChangeRequest.isPending}
              helperText="Internal only — never shown to the customer."
            />

            <AsyncEntitySelect
              label="Service"
              placeholder="Search services…"
              value={service}
              onChange={(next) => {
                setService(next);
                // A service offering only makes sense under its own service — drop it rather than
                // leave a stale pairing.
                setServiceOffering(null);
              }}
              disabled={createChangeRequest.isPending}
              search={changeRequestLookups.itServices}
            />
            <AsyncEntitySelect
              label="Service offering"
              placeholder="Search service offerings…"
              value={serviceOffering}
              onChange={setServiceOffering}
              disabled={createChangeRequest.isPending}
              search={changeRequestLookups.serviceOfferings}
              searchExtra={service?.id}
              helperText={service ? undefined : "Narrows to a Service once one is picked."}
            />
            <AsyncEntitySelect
              label="Configuration item"
              placeholder="Search configuration items…"
              value={configurationItem}
              onChange={setConfigurationItem}
              disabled={createChangeRequest.isPending}
              search={changeRequestLookups.configurationItems}
            />
            <AsyncEntitySelect
              label="Assignment group"
              placeholder="Search groups…"
              value={group}
              onChange={setGroup}
              disabled={createChangeRequest.isPending}
              search={changeRequestLookups.groups}
            />
            <AsyncEntitySelect
              label="Assigned to"
              placeholder="Search people…"
              value={assignedEngineer}
              onChange={setAssignedEngineer}
              disabled={createChangeRequest.isPending}
              search={changeRequestLookups.users}
            />
            <AsyncEntitySelect
              label="Requested by"
              placeholder="Search people…"
              value={requestedBy}
              onChange={setRequestedBy}
              disabled={createChangeRequest.isPending}
              search={changeRequestLookups.users}
              helperText="Defaults to you — clear it if this wasn't your request."
            />
          </AccordionDetails>
        </Accordion>

        {submitError && (
          <Typography variant="body2" color="error.main">
            {submitError}
          </Typography>
        )}

        <Stack direction="row" gap={1} justifyContent="flex-end">
          {/* navigate(-1) rather than a hardcoded /operations path — this app's Operations tab
              state isn't URL-driven, so going back preserves whichever tab (Change Requests) the
              Fab was pressed from, same as NewServiceRequestPage.tsx's own Cancel button. */}
          <Button onClick={() => navigate(-1)} disabled={createChangeRequest.isPending}>
            Cancel
          </Button>
          <Button variant="contained" disabled={!canSubmit} onClick={handleSubmit}>
            {createChangeRequest.isPending ? "Creating…" : "Create Change Request"}
          </Button>
        </Stack>
      </Stack>
    </Stack>
  );
}
