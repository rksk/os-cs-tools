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

import {
  AdapterDateFns,
  Box,
  Button,
  CircularProgress,
  DatePickers,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  InputLabel,
  MenuItem,
  Select,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { useState, type JSX } from "react";
import { formatDateOnly, isPastDateOnly, parseDateOnly } from "@utils/dateTime";
import { useSearchProducts } from "@features/csm-projects/api/useSearchProducts";

const { DesktopDatePicker: DatePicker, LocalizationProvider } = DatePickers;

/**
 * True only for an absolute http(s) URL that points at a GitHub issue or pull
 * request (`github.com/<owner>/<repo>/issues|pull/<number>`) — digiops-cs#3449
 * asked specifically for GitHub-URL validation on this field, not a plain
 * "looks like a URL" check (see `@utils/isHttpUrl` for that narrower, purely
 * security-motivated one used elsewhere).
 */
function isGithubIssueUrl(value: string): boolean {
  try {
    const url = new URL(value);
    if (url.protocol !== "https:" && url.protocol !== "http:") return false;
    if (url.hostname.toLowerCase() !== "github.com") return false;
    return /^\/[^/]+\/[^/]+\/(issues|pull)\/\d+\/?$/.test(url.pathname);
  } catch {
    return false;
  }
}

/** Combined `PATCH /cases/{id}` payload this dialog can produce in one save. */
export interface FixEtaSavePayload {
  bestCaseFixEta?: string;
  mostLikelyFixEta?: string;
  worstCaseFixEta?: string;
  addPublicComment?: boolean;
  product?: string;
  publicTicket?: string;
}

interface SetFixEtaDialogProps {
  /**
   * Reason the "Share fix ETA with customer" toggle is currently unavailable
   * — the share now posts through the same `POST /cases/{id}/comments`
   * endpoint (and the identical work_in_progress + ongoing + assignee gate)
   * as the case's own public-reply composer (`CsmCaseCommentInput`'s
   * `publicCommentDisabledReason`), so a share attempt that doesn't satisfy
   * it is guaranteed to fail after the ETA has already saved. `null`/absent
   * means sharing is currently available. Never applies to the ETA dates
   * themselves — those save regardless of this gate.
   */
  publicCommentDisabledReason?: string | null;
  /** Current internal-only best-case estimate, if any (date-only "YYYY-MM-DD"). */
  currentBestCaseFixEta?: string | null;
  /** Current internal-only most-likely estimate, if any (date-only "YYYY-MM-DD"). */
  currentMostLikelyFixEta?: string | null;
  /** Current internal-only worst-case estimate, if any (date-only "YYYY-MM-DD"). */
  currentWorstCaseFixEta?: string | null;
  /** True while the combined PATCH is in flight; disables the whole form. */
  isSaving: boolean;
  onClose: () => void;
  /** Apply the combined patch in one `PATCH` call. */
  onSave: (patch: FixEtaSavePayload) => void;
}

interface FixEtaDatePickerProps {
  label: string;
  value: string;
  disabled: boolean;
  onChange: (valueDateOnly: string) => void;
  /**
   * A blocking validation message for this specific field (a past date, or an
   * ordering conflict with one of the other two estimates) — takes priority
   * over the field's own past-date check, since an ordering error is more
   * specific/actionable than a generic "this is in the past" when both apply.
   */
  errorText?: string;
}

/**
 * One date-only picker in the combined form. Unlike the old per-row layout,
 * this no longer owns a Save action — the whole dialog saves together.
 */
function FixEtaDatePicker({
  label,
  value,
  disabled,
  onChange,
  errorText,
}: FixEtaDatePickerProps): JSX.Element {
  const parsed = parseDateOnly(value);
  // A past estimate is now a hard block, not just a warning (digiops-cs#3449:
  // "ETA fields can set past days" was reported as a bug) -- fix-ETA dates are
  // forward-looking commitments, so a date already in the past can never be a
  // genuine estimate being made today.
  const isPast = isPastDateOnly(parsed);
  const helperText = errorText ?? (isPast ? "This date is in the past." : undefined);

  return (
    <LocalizationProvider dateAdapter={AdapterDateFns}>
      <DatePicker
        label={label}
        value={parsed}
        onChange={(next) =>
          onChange(
            next instanceof Date && !Number.isNaN(next.getTime())
              ? formatDateOnly(next)
              : "",
          )
        }
        disabled={disabled}
        slotProps={{
          textField: {
            fullWidth: true,
            size: "small",
            error: !!errorText || isPast,
            helperText,
          },
        }}
      />
    </LocalizationProvider>
  );
}

/**
 * Set the case's three independent internal-only fix-ETA estimates
 * (`bestCaseFixEta` / `mostLikelyFixEta` / `worstCaseFixEta`) and, optionally,
 * share them with the customer as a real comment (the caller posts this via
 * `POST /cases/{id}/comments`, not a PATCH field — see `onSave`'s own
 * `FixEtaSavePayload`). The three dates remain independently optional —
 * saving one doesn't require the others — but they now share a single Save
 * action instead of three. Sharing itself is gated by
 * `publicCommentDisabledReason`, the same work_in_progress + ongoing +
 * assignee rule the comment endpoint enforces for any public comment.
 */
export default function SetFixEtaDialog({
  publicCommentDisabledReason = null,
  currentBestCaseFixEta,
  currentMostLikelyFixEta,
  currentWorstCaseFixEta,
  isSaving,
  onClose,
  onSave,
}: SetFixEtaDialogProps): JSX.Element {
  const canShareWithCustomer = publicCommentDisabledReason === null;
  const [bestCaseFixEta, setBestCaseFixEta] = useState(
    currentBestCaseFixEta ?? "",
  );
  const [mostLikelyFixEta, setMostLikelyFixEta] = useState(
    currentMostLikelyFixEta ?? "",
  );
  const [worstCaseFixEta, setWorstCaseFixEta] = useState(
    currentWorstCaseFixEta ?? "",
  );
  const [shareWithCustomer, setShareWithCustomer] = useState(false);
  const [productId, setProductId] = useState("");
  const [publicTicket, setPublicTicket] = useState("");
  const { data: products, isLoading: productsLoading } = useSearchProducts();

  const hasAnyEta = !!(bestCaseFixEta || mostLikelyFixEta || worstCaseFixEta);
  // Derived from a real catalogue match with a real name, not just a
  // non-empty productId -- if the selected product disappears from the
  // fetched list (a refetch mid-dialog) or carries no name (BeProduct.name
  // is optional), productId would otherwise still read as "set" and either
  // the raw UUID or an empty string could leak into the customer-visible
  // comment via a stale fallback.
  const selectedProduct = products?.find((p) => p.id === productId);
  const hasProduct = !!selectedProduct?.name;
  const hasPublicTicket = publicTicket.trim().length > 0;
  const trimmedPublicTicket = publicTicket.trim();
  const isPublicTicketValidUrl =
    !hasPublicTicket || isGithubIssueUrl(trimmedPublicTicket);

  // Past-date and ordering checks run regardless of whether sharing is on --
  // an internal-only estimate with a date already in the past, or with the
  // three out of best <= most likely <= worst case order, is just as wrong
  // when nobody is sharing it with the customer (digiops-cs#3449).
  const bestParsed = parseDateOnly(bestCaseFixEta);
  const mostLikelyParsed = parseDateOnly(mostLikelyFixEta);
  const worstParsed = parseDateOnly(worstCaseFixEta);
  const bestIsPast = isPastDateOnly(bestParsed);
  const mostLikelyIsPast = isPastDateOnly(mostLikelyParsed);
  const worstIsPast = isPastDateOnly(worstParsed);
  const hasPastDate = bestIsPast || mostLikelyIsPast || worstIsPast;

  const bestAfterMostLikely =
    !!bestParsed && !!mostLikelyParsed && bestParsed.getTime() > mostLikelyParsed.getTime();
  const mostLikelyAfterWorst =
    !!mostLikelyParsed && !!worstParsed && mostLikelyParsed.getTime() > worstParsed.getTime();
  const bestAfterWorst =
    !!bestParsed && !!worstParsed && bestParsed.getTime() > worstParsed.getTime();
  const hasOrderingError = bestAfterMostLikely || mostLikelyAfterWorst || bestAfterWorst;

  const bestCaseErrorText = bestAfterMostLikely
    ? "Best case must not be later than Most likely."
    : bestAfterWorst
      ? "Best case must not be later than Worst case."
      : undefined;
  const mostLikelyErrorText = mostLikelyAfterWorst
    ? "Most likely must not be later than Worst case."
    : undefined;

  // Checked first: sharing posts a real customer-visible comment, gated by
  // the same work_in_progress + ongoing + assignee rule as the case's own
  // public-reply composer — a share attempt that doesn't satisfy it is
  // guaranteed to fail (after the ETA has already saved), so this is caught
  // here rather than only discovered from the resulting error toast. The
  // rest mirrors the backend's own validation for the share fields
  // themselves: a public comment must summarize at least one estimate, and
  // needs a product + ticket reference to be meaningful to the customer.
  const shareValidationError = shareWithCustomer
    ? !canShareWithCustomer
      ? publicCommentDisabledReason
      : !hasAnyEta
        ? "Pick at least one fix ETA to share with the customer."
        : !hasProduct
          ? "Product is required to share with the customer."
          : !hasPublicTicket
            ? "Public ticket is required to share with the customer."
            : !isPublicTicketValidUrl
              ? "Public ticket must be a GitHub issue or pull request URL."
              : undefined
    : !hasPublicTicket || isPublicTicketValidUrl
      ? undefined
      : "Public ticket must be a GitHub issue or pull request URL.";

  const dateValidationError = hasPastDate
    ? "Fix ETA dates can't be in the past."
    : hasOrderingError
      ? "Best case must be no later than Most likely, which must be no later than Worst case."
      : undefined;

  const canSubmit =
    (hasAnyEta || shareWithCustomer) &&
    !shareValidationError &&
    !dateValidationError;

  const handleSave = (): void => {
    if (!canSubmit) return;
    // canSubmit already requires hasProduct (a real selectedProduct match)
    // whenever shareWithCustomer is on, so selectedProduct is never
    // undefined here -- no raw productId fallback, ever.
    onSave({
      ...(bestCaseFixEta && { bestCaseFixEta }),
      ...(mostLikelyFixEta && { mostLikelyFixEta }),
      ...(worstCaseFixEta && { worstCaseFixEta }),
      ...(shareWithCustomer && {
        addPublicComment: true,
        product: selectedProduct?.name ?? "",
        publicTicket: trimmedPublicTicket,
      }),
    });
  };

  return (
    <Dialog open onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Set fix ETA</DialogTitle>
      <DialogContent>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 2, pt: 0.5 }}>
          <Typography variant="body2" color="text.secondary">
            Internal-only estimates. Fill in as many as you have — none are
            required on their own — then save them together.
          </Typography>

          <FixEtaDatePicker
            label="Best case"
            value={bestCaseFixEta}
            disabled={isSaving}
            onChange={setBestCaseFixEta}
            errorText={
              bestIsPast ? "This date is in the past." : bestCaseErrorText
            }
          />
          <FixEtaDatePicker
            label="Most likely"
            value={mostLikelyFixEta}
            disabled={isSaving}
            onChange={setMostLikelyFixEta}
            errorText={
              mostLikelyIsPast ? "This date is in the past." : mostLikelyErrorText
            }
          />
          <FixEtaDatePicker
            label="Worst case"
            value={worstCaseFixEta}
            disabled={isSaving}
            onChange={setWorstCaseFixEta}
            errorText={worstIsPast ? "This date is in the past." : undefined}
          />

          <FormControlLabel
            control={
              <Switch
                checked={shareWithCustomer}
                onChange={(e) => setShareWithCustomer(e.target.checked)}
                disabled={isSaving || !canShareWithCustomer}
              />
            }
            label="Share fix ETA with customer"
          />
          {!canShareWithCustomer && (
            <Typography variant="caption" color="text.secondary">
              {publicCommentDisabledReason}
            </Typography>
          )}

          {shareWithCustomer && (
            <>
              <Typography variant="caption" color="text.secondary">
                Posts a customer-visible comment on this case summarizing the
                estimate(s) above.
              </Typography>
              <FormControl size="small" fullWidth required>
                <InputLabel id="set-fix-eta-product-label" shrink={productId !== ""}>
                  Product
                </InputLabel>
                <Select
                  labelId="set-fix-eta-product-label"
                  label="Product"
                  value={productId}
                  onChange={(e) => setProductId(e.target.value as string)}
                  notched={productId !== ""}
                  disabled={isSaving}
                  startAdornment={
                    productsLoading ? (
                      <CircularProgress size={16} sx={{ mr: 1 }} />
                    ) : null
                  }
                >
                  {(products ?? []).map((p) => (
                    <MenuItem key={p.id} value={p.id}>
                      {p.name ?? p.id}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
              <TextField
                label="Public ticket"
                placeholder="e.g. a public GitHub issue URL"
                size="small"
                fullWidth
                required
                value={publicTicket}
                onChange={(e) => setPublicTicket(e.target.value)}
                disabled={isSaving}
                error={hasPublicTicket && !isPublicTicketValidUrl}
                helperText={
                  hasPublicTicket && !isPublicTicketValidUrl
                    ? "Must be a GitHub issue or pull request URL."
                    : undefined
                }
              />
            </>
          )}

          {shareValidationError && (
            <Typography variant="caption" color="error">
              {shareValidationError}
            </Typography>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={isSaving}>
          Close
        </Button>
        <Button
          variant="contained"
          disabled={!canSubmit || isSaving}
          loading={isSaving}
          onClick={handleSave}
        >
          Save
        </Button>
      </DialogActions>
    </Dialog>
  );
}
