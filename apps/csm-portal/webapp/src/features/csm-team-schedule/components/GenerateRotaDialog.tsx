/**
 * Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { useMemo, useState, type JSX } from "react";
import { useGenerateRota, type GenerateRotaResult } from "../api/useGenerateRota";
import GeneratedRotaPreview from "./GeneratedRotaPreview";
import { monthKey, monthLabel } from "../utils/rotaMonth";

/** How many of the generator's notes are listed before the rest are counted. */
const SHOWN_WARNINGS = 8;

/** One team of the generated rota, for naming it in the preview. */
export interface GenerateRotaTeam {
  key: string;
  name: string;
}

/** One move onto the SaaS Night cover: who, from which team, for which days.
 *  An open-ended move has no end. */
export interface NightCoverSpan {
  userId: string;
  name: string;
  homeTeam: string;
  startsOn: string;
  endsOn?: string;
}

export interface GenerateRotaDialogProps {
  open: boolean;
  onClose: () => void;
  /** Today, for the month choices. Injected so a test can fix it. */
  today?: Date;
  /** The rota's teams, for naming them in the preview. */
  teams?: readonly GenerateRotaTeam[];
  /** Everybody moved onto the SaaS Night cover over the months on offer: the
   *  month's TZ3 crew is whoever of them it covers. */
  nightCover?: readonly NightCoverSpan[];
}

/**
 * The SaaS rota's "Generate month": pick a month, preview what the generator
 * would write from the availability marked on the roster, then write it.
 * Turns a lead set by hand are never replaced, and generating a month again
 * replaces only the generated turns from today on.
 */
export default function GenerateRotaDialog({
  open,
  onClose,
  today = new Date(),
  teams = [],
  nightCover = [],
}: GenerateRotaDialogProps): JSX.Element {
  const months = useMemo(() => [0, 1, 2].map((n) => monthKey(today, n)), [today]);
  const [month, setMonth] = useState(months[1]);
  const [preview, setPreview] = useState<GenerateRotaResult | null>(null);
  const [written, setWritten] = useState<GenerateRotaResult | null>(null);
  const generate = useGenerateRota();
  /** The month's TZ3 crew: everybody a move puts on the SaaS Night cover on
   *  any day of it, once each, in name order. Who is on it is changed on the
   *  roster (Move to SaaS Night cover), not here. */
  const crew = useMemo(() => {
    const first = `${month}-01`;
    const [y, m] = month.split("-").map(Number);
    const last = `${month}-${String(new Date(y, m, 0).getDate()).padStart(2, "0")}`;
    const seen = new Map<string, NightCoverSpan>();
    for (const span of nightCover) {
      if (span.startsOn > last || (span.endsOn !== undefined && span.endsOn < first)) continue;
      if (!seen.has(span.userId)) seen.set(span.userId, span);
    }
    return [...seen.values()].sort((a, b) => a.name.localeCompare(b.name));
  }, [nightCover, month]);
  const [showRota, setShowRota] = useState(false);
  /** A real write is in flight. The request cannot be called back, so the
   *  dialog stays open until it lands: closing now would only hide a month
   *  that is still being written. A preview writes nothing and may be left. */
  const writing = generate.isPending && generate.variables?.dryRun !== true;

  const reset = (): void => {
    setPreview(null);
    setWritten(null);
    setShowRota(false);
    generate.reset();
  };
  const close = (): void => {
    if (writing) return;
    reset();
    onClose();
  };

  const runPreview = (): void => {
    setWritten(null);
    generate.mutate({ month, dryRun: true }, { onSuccess: (res) => setPreview(res) });
  };
  const runGenerate = (): void => {
    if (!preview) return;
    generate.mutate(
      { month, regenerate: preview.alreadyGenerated },
      { onSuccess: (res) => setWritten(res) },
    );
  };

  const shown = written ?? preview;
  const warnings = shown?.warnings ?? [];

  return (
    <Dialog
      open={open}
      onClose={close}
      disableEscapeKeyDown={writing}
      maxWidth="md"
      fullWidth
      aria-labelledby="generate-rota-title"
    >
      <DialogTitle id="generate-rota-title">Generate SaaS rota</DialogTitle>
      <DialogContent>
        <Typography variant="body2" sx={{ mb: 2 }}>
          Works out the month for Apollo and Artemis: per team, L1 and L2 for TZ1 and TZ2 every day
          (weekend on-call and escalation included); one TZ3 night crew for both teams, L1, L2 and L3,
          weekends included; everyone&rsquo;s regular hours (SUP) on weekdays; and lieu leave after a
          weekend on-call. Every
          engineer is on the rota, nights included, unless marked away on the roster (leave, RnD, ENG or
          any other tag). Nights go to the people on the SaaS Night cover that month. One person works
          one zone a day: each team&rsquo;s lead is L3 on TZ1, and a teammate on TZ2. Turns set by hand
          are kept.
        </Typography>

        <TextField
          select
          label="Month"
          value={month}
          onChange={(e) => {
            setMonth(e.target.value);
            reset();
          }}
          size="small"
          fullWidth
          disabled={generate.isPending || written !== null}
        >
          {months.map((m) => (
            <MenuItem key={m} value={m}>
              {monthLabel(m)}
            </MenuItem>
          ))}
        </TextField>

        <Box sx={{ mt: 2 }}>
          <Typography variant="subtitle2">TZ3 night crew</Typography>
          {crew.length > 0 ? (
            <>
              <Typography variant="caption" color="text.secondary" component="p" sx={{ mt: 0.25, mb: 1 }}>
                The people on the SaaS Night cover in {monthLabel(month)}. L1, L2 and L3 go round them night by
                night, weekends included, and they work no other zone while they are on it. To change who is
                on it, move people on or off the SaaS Night cover on the roster.
              </Typography>
              <Box component="ul" sx={{ m: 0, pl: 2.5 }}>
                {crew.map((p) => (
                  <Typography key={p.userId} component="li" variant="body2">
                    {p.name} <Typography component="span" variant="caption" color="text.secondary">({p.homeTeam})</Typography>
                  </Typography>
                ))}
              </Box>
              {crew.length < 3 ? (
                <Typography variant="caption" color="warning.main" component="p" sx={{ mt: 1 }}>
                  Only {crew.length} on the Night cover: TZ3 {crew.length === 1 ? "L2 and L3 are" : "L3 is"} left empty every night.
                </Typography>
              ) : null}
            </>
          ) : (
            <Typography variant="caption" color="text.secondary" component="p" sx={{ mt: 0.25 }}>
              Nobody is on the SaaS Night cover in {monthLabel(month)}. Nights go to the people with TZ3 regular
              hours marked on the roster, or the engineers take turns. To put people on nights for the coming
              months, use Move to SaaS Night cover on the roster.
            </Typography>
          )}
        </Box>

        {generate.isError ? (
          <Alert severity="error" sx={{ mt: 2 }}>
            {generate.error.message || "The rota could not be generated."}
          </Alert>
        ) : null}

        {shown ? (
          <Box sx={{ mt: 2 }} role="status">
            {written ? (
              <Alert severity="success">
                {monthLabel(written.month)} written from {written.from}: {written.summary.written} turns and SUP
                {written.summary.replaced > 0 ? `, ${written.summary.replaced} earlier generated turns replaced` : ""}
                {written.summary.lieuWritten > 0 ? `, ${written.summary.lieuWritten} lieu leave` : ""}.
              </Alert>
            ) : (
              <>
                <Typography variant="body2">
                  <b>{shown.summary.planned - shown.summary.keptByHand}</b> turns and SUP from {shown.from}
                  {shown.summary.keptByHand > 0
                    ? `, ${shown.summary.keptByHand} left as set by hand`
                    : ""}
                  {shown.summary.lieuPlanned > 0 ? `, ${shown.summary.lieuPlanned} lieu leave` : ""}.
                </Typography>
                {shown.alreadyGenerated ? (
                  <Alert severity="info" sx={{ mt: 1 }}>
                    This month has already been generated. Generating again replaces the generated turns
                    from {shown.from} on; turns set by hand are kept.
                  </Alert>
                ) : null}
                <Button size="small" sx={{ mt: 1, px: 0 }} onClick={() => setShowRota((v) => !v)} aria-expanded={showRota}>
                  {showRota ? "Hide who works when" : "Show who works when"}
                </Button>
                {showRota ? (
                  <GeneratedRotaPreview
                    result={shown}
                    teamNames={Object.fromEntries(teams.map((t) => [t.key, t.name]))}
                  />
                ) : null}
              </>
            )}
            {warnings.length > 0 ? (
              <Alert severity="warning" sx={{ mt: 1 }}>
                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                  {warnings.length} slot{warnings.length === 1 ? "" : "s"} to look at
                </Typography>
                <Box component="ul" sx={{ m: 0, pl: 2 }}>
                  {warnings.slice(0, SHOWN_WARNINGS).map((w, i) => (
                    <li key={i}>
                      {w.date ? `${w.date}: ` : ""}
                      {w.message}
                    </li>
                  ))}
                </Box>
                {warnings.length > SHOWN_WARNINGS ? (
                  <Typography variant="caption">and {warnings.length - SHOWN_WARNINGS} more</Typography>
                ) : null}
              </Alert>
            ) : null}
          </Box>
        ) : null}
      </DialogContent>
      <DialogActions>
        {writing ? (
          <Typography variant="body2" color="text.secondary" role="status" sx={{ mr: "auto", pl: 1 }}>
            Writing the month&hellip;
          </Typography>
        ) : null}
        <Button onClick={close} disabled={writing}>
          {written ? "Done" : "Cancel"}
        </Button>
        {written ? null : preview ? (
          <Button variant="contained" onClick={runGenerate} disabled={generate.isPending}>
            {preview.alreadyGenerated ? "Regenerate" : "Generate"}
          </Button>
        ) : (
          <Button variant="contained" onClick={runPreview} disabled={generate.isPending}>
            Preview
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
}
