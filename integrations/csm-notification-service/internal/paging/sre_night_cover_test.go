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

package paging

import (
	"context"
	"strings"
	"testing"
	"time"
)

// SaaS SRE's night cover: whoever holds SRE_TZ3 (21:00-06:00 IST, every day)
// is ONE crew for Apollo and Artemis together, one person per tier for the
// whole rota. entity-service refuses a second holder of a TZ3 tier across the
// two teams (migration 0223), so the pager sees one L1, one L2 and one L3 on
// a night, from either team. These tests pin that every SaaS incident at
// night -- Apollo's, Artemis's, or one with no team -- climbs that crew, and
// that the night begins and ends on the window's own hours.
//
// The stub rota below is time-aware, unlike stubScheduleReader: a turn is on
// duty over [start, end) of its window on its rota day, which is how
// entity-service's OnDutyAt matches an instant (tstzrange(starts_at, ends_at,
// '[)')). That is what makes 05:59 the previous night's crew and 06:00 the
// morning's.

// sreWindowHours are the SaaS SRE windows' hours as migration 0154 seeds
// them, in minutes from midnight IST on the rota day; an end past 1440 runs
// into the next day.
var sreWindowHours = map[string]struct {
	zone       string
	start, end int
}{
	"SRE_TZ1_L1":      {"TZ1", 360, 810},
	"SRE_TZ1":         {"TZ1", 360, 810},
	"SRE_TZ2_L1":      {"TZ2", 810, 1260},
	"SRE_TZ2":         {"TZ2", 810, 1260},
	"SRE_TZ3":         {"TZ3", 1260, 1800},
	"SRE_TZ3_REGULAR": {"TZ3", 1260, 1800},
	"SRE_WE_TZ1":      {"TZ1", 360, 1260},
}

// nightCoverCatalogue is the SaaS rota as a rota-aware entity-service serves
// it: Apollo and Artemis on SRE_SAAS, TZ1-TZ3 its zones, and its windows --
// TZ3's regular hours included, which carry no tier.
func nightCoverCatalogue() *scheduleCatalogue {
	saas := optStr(RotaSRESaaS)
	cat := &scheduleCatalogue{}
	for _, team := range []string{"apollo", "artemis"} {
		cat.Teams = append(cat.Teams, catalogueTeam{Key: team, Name: team, Family: familySRE, RotaCode: saas})
	}
	for _, zone := range []string{"TZ1", "TZ2", "TZ3"} {
		cat.Zones = append(cat.Zones, catalogueZone{Code: zone, RotaCode: saas})
	}
	for _, sh := range []struct{ code, tier string }{
		{"SRE_TZ1_L1", "L1"}, {"SRE_TZ1", ""}, {"SRE_TZ2_L1", "L1"}, {"SRE_TZ2", ""},
		{"SRE_TZ3", ""}, {"SRE_TZ3_REGULAR", ""}, {"SRE_WE_TZ1", ""},
	} {
		cat.Shifts = append(cat.Shifts, catalogueShift{
			Code: sh.code, Family: familySRE, ZoneCode: optStr(sreWindowHours[sh.code].zone), Tier: optStr(sh.tier),
		})
	}
	return cat
}

// rotaTurn is one assignment with the instants its window resolves to.
type rotaTurn struct {
	onDutyAssignment
	starts, ends time.Time
}

// turnOn is user on a SaaS window for team on rotaDay (YYYY-MM-DD). tier ""
// takes the window's own, as held does.
func turnOn(t *testing.T, user, team, shift, tier, rotaDay string) rotaTurn {
	t.Helper()
	w, ok := sreWindowHours[shift]
	if !ok {
		t.Fatalf("no hours for window %s", shift)
	}
	day, err := time.ParseInLocation("2006-01-02", rotaDay, IST)
	if err != nil {
		t.Fatalf("rota day %q: %v", rotaDay, err)
	}
	a := held(user, team, shift, tier)
	a.ZoneCode = optStr(w.zone)
	return rotaTurn{
		onDutyAssignment: a,
		starts:           day.Add(time.Duration(w.start) * time.Minute),
		ends:             day.Add(time.Duration(w.end) * time.Minute),
	}
}

// clockedRota answers OnDutyAt with the turns live at the instant.
type clockedRota struct {
	*stubScheduleReader
	turns []rotaTurn
}

func (c clockedRota) OnDutyAt(_ context.Context, at time.Time, _ bool) ([]onDutyAssignment, error) {
	var live []onDutyAssignment
	for _, tr := range c.turns {
		if !at.Before(tr.starts) && at.Before(tr.ends) {
			live = append(live, tr.onDutyAssignment)
		}
	}
	return live, nil
}

// nightCoverWeek is a week of the SaaS rota with a different night crew each
// night, mixed across the two teams, and both teams' day crews around them,
// so a rung that read the wrong night or the wrong zone names somebody else.
//
//	Tue 13  night  L1 ap-n1  L2 ar-n2  L3 ap-n3   (+ ar-reg on TZ3 regular hours)
//	Wed 14  TZ1    L1 ap-d1 / ar-d1, L2 ap-d2 / ar-d2
//	        TZ2    L1 ap-e1 / ar-e1
//	        night  L1 ar-m1  L2 ap-m2  L3 ar-m3
//	Thu 15  TZ1    L1 ap-t1 / ar-t1 (no L2 or L3)
//	        night  L1 ap-x1
//	Fri 16  night  L1 ap-f1  L2 ar-f2  L3 ap-f3
//	Sat 17  WE     L1 ap-we1 / ar-we1 (06:00-21:00)
//	        night  L1 ar-s1  L2 ap-s2  L3 ar-s3
//	Sun 18  WE     L1 ap-su1
//	        night  L1 ap-u1
func nightCoverWeek(t *testing.T) TeamScheduleResolver {
	t.Helper()
	turns := []rotaTurn{
		turnOn(t, "ap-n1", "apollo", "SRE_TZ3", "L1", "2026-10-13"),
		turnOn(t, "ar-n2", "artemis", "SRE_TZ3", "L2", "2026-10-13"),
		turnOn(t, "ap-n3", "apollo", "SRE_TZ3", "L3", "2026-10-13"),
		turnOn(t, "ar-reg", "artemis", "SRE_TZ3_REGULAR", "", "2026-10-13"),

		turnOn(t, "ap-d1", "apollo", "SRE_TZ1_L1", "", "2026-10-14"),
		turnOn(t, "ar-d1", "artemis", "SRE_TZ1_L1", "", "2026-10-14"),
		turnOn(t, "ap-d2", "apollo", "SRE_TZ1", "L2", "2026-10-14"),
		turnOn(t, "ar-d2", "artemis", "SRE_TZ1", "L2", "2026-10-14"),
		turnOn(t, "ap-e1", "apollo", "SRE_TZ2_L1", "", "2026-10-14"),
		turnOn(t, "ar-e1", "artemis", "SRE_TZ2_L1", "", "2026-10-14"),
		turnOn(t, "ar-m1", "artemis", "SRE_TZ3", "L1", "2026-10-14"),
		turnOn(t, "ap-m2", "apollo", "SRE_TZ3", "L2", "2026-10-14"),
		turnOn(t, "ar-m3", "artemis", "SRE_TZ3", "L3", "2026-10-14"),

		turnOn(t, "ap-t1", "apollo", "SRE_TZ1_L1", "", "2026-10-15"),
		turnOn(t, "ar-t1", "artemis", "SRE_TZ1_L1", "", "2026-10-15"),
		turnOn(t, "ap-x1", "apollo", "SRE_TZ3", "L1", "2026-10-15"),

		turnOn(t, "ap-f1", "apollo", "SRE_TZ3", "L1", "2026-10-16"),
		turnOn(t, "ar-f2", "artemis", "SRE_TZ3", "L2", "2026-10-16"),
		turnOn(t, "ap-f3", "apollo", "SRE_TZ3", "L3", "2026-10-16"),

		turnOn(t, "ap-we1", "apollo", "SRE_WE_TZ1", "L1", "2026-10-17"),
		turnOn(t, "ar-we1", "artemis", "SRE_WE_TZ1", "L1", "2026-10-17"),
		turnOn(t, "ar-s1", "artemis", "SRE_TZ3", "L1", "2026-10-17"),
		turnOn(t, "ap-s2", "apollo", "SRE_TZ3", "L2", "2026-10-17"),
		turnOn(t, "ar-s3", "artemis", "SRE_TZ3", "L3", "2026-10-17"),

		turnOn(t, "ap-su1", "apollo", "SRE_WE_TZ1", "L1", "2026-10-18"),
		turnOn(t, "ap-u1", "apollo", "SRE_TZ3", "L1", "2026-10-18"),
	}
	return NewTeamScheduleResolver(clockedRota{stubScheduleReader: &stubScheduleReader{catalogue: nightCoverCatalogue()}, turns: turns}, sreTeams, nil)
}

// nightIncidents are the three kinds of SaaS incident a night crew answers:
// Artemis's, Apollo's, and one with no team (which pages the default rota,
// SaaS).
var nightIncidents = []string{"Artemis", "Apollo", ""}

// ladderAt resolves L1, L2 and L3 for an incident of team at the instant, one
// user name per rung ("" for a rung nobody answers).
func ladderAt(t *testing.T, r TeamScheduleResolver, team string, at time.Time) [3]string {
	t.Helper()
	var out [3]string
	for i, level := range []Level{Level0, Level1, Level2} {
		got, err := r.Resolve(context.Background(), level, RoutingContext{AssignedCRETeam: team, Ladder: LadderSRE, At: at})
		if err != nil {
			t.Fatalf("resolve %s for %q at %s: %v", level, team, at, err)
		}
		if len(got) > 1 {
			t.Fatalf("%s for %q at %s reached %v; every SRE rung is one person", level, team, at, emails(got))
		}
		if len(got) == 1 {
			out[i] = strings.TrimSuffix(got[0].Email, "@example.com")
		}
	}
	return out
}

// expectCrew checks every night incident climbs the same crew at the instant.
func expectCrew(t *testing.T, r TeamScheduleResolver, label string, at time.Time, want [3]string) {
	t.Helper()
	for _, team := range nightIncidents {
		if got := ladderAt(t, r, team, at); got != want {
			t.Errorf("%s, incident of team %q: L1/L2/L3 = %v, want %v", label, team, got, want)
		}
	}
}

// 02:00 on a weekday is the previous evening's TZ3 crew, whichever team the
// incident is on: an Apollo L1 for an Artemis incident, an Artemis L2 for an
// Apollo one. Own-team preference cannot pull anybody else in, because the
// night has one holder per tier.
func TestResolveSRE_NightCoverIsOneCrewForBothTeams(t *testing.T) {
	r := nightCoverWeek(t)
	expectCrew(t, r, "Wed 02:00 IST (Tuesday's night)", ist(2026, 10, 14, 2, 0), [3]string{"ap-n1", "ar-n2", "ap-n3"})
}

// The weekend night is SRE_TZ3 too (day scope ANY): Saturday 23:00 and Sunday
// 02:00 are both Saturday's crew, not the weekend day crew, Friday's night or
// Sunday's.
func TestResolveSRE_WeekendNightCoverIsSaturdaysCrew(t *testing.T) {
	r := nightCoverWeek(t)
	saturday := [3]string{"ar-s1", "ap-s2", "ar-s3"}
	expectCrew(t, r, "Sat 23:00 IST", ist(2026, 10, 17, 23, 0), saturday)
	expectCrew(t, r, "Sun 02:00 IST", ist(2026, 10, 18, 2, 0), saturday)
}

// The night starts at 21:00 and ends at 06:00. 20:59 is still TZ2, each team
// its own L1; 21:00 is the night crew; 05:59 the next morning is still that
// night's crew; 06:00 on a weekday is TZ1 again, each team its own L1, and the
// night's L2 and L3 are no longer reached.
func TestResolveSRE_NightCoverStartsAt21AndEndsAt06(t *testing.T) {
	r := nightCoverWeek(t)

	evening := ist(2026, 10, 14, 20, 59)
	for team, want := range map[string]string{"Artemis": "ar-e1", "Apollo": "ap-e1", "": "ap-e1"} {
		if got := ladderAt(t, r, team, evening)[0]; got != want {
			t.Errorf("Wed 20:59 (TZ2), incident of team %q: L1 = %q, want %q", team, got, want)
		}
	}

	wednesdayNight := [3]string{"ar-m1", "ap-m2", "ar-m3"}
	expectCrew(t, r, "Wed 21:00 IST", ist(2026, 10, 14, 21, 0), wednesdayNight)
	expectCrew(t, r, "Thu 05:59 IST", ist(2026, 10, 15, 5, 59), wednesdayNight)

	morning := ist(2026, 10, 15, 6, 0)
	for team, want := range map[string][3]string{
		"Artemis": {"ar-t1", "", ""},
		"Apollo":  {"ap-t1", "", ""},
		"":        {"ap-t1", "", ""}, // no team: the configured team order
	} {
		if got := ladderAt(t, r, team, morning); got != want {
			t.Errorf("Thu 06:00 (TZ1), incident of team %q: L1/L2/L3 = %v, want %v", team, got, want)
		}
	}
	got, err := r.Resolve(context.Background(), Level0, RoutingContext{AssignedCRETeam: "Artemis", Ladder: LadderSRE, At: morning})
	if err != nil || len(got) != 1 || got[0].ShiftCode != "SRE_TZ1_L1" {
		t.Fatalf("Thu 06:00 L1 = %+v, %v; want one holder on SRE_TZ1_L1, not SRE_TZ3", got, err)
	}
}

// TZ3's regular hours carry no tier, so an Artemis engineer on them is not a
// holder of anything: an Artemis incident's L1 is still the night crew's
// Apollo L1, and a tier nobody holds stays empty rather than falling to them.
func TestResolveSRE_TZ3RegularHoursHoldNoTier(t *testing.T) {
	rota := clockedRota{
		stubScheduleReader: &stubScheduleReader{catalogue: nightCoverCatalogue()},
		turns: []rotaTurn{
			turnOn(t, "ap-n1", "apollo", "SRE_TZ3", "L1", "2026-10-13"),
			turnOn(t, "ar-reg", "artemis", "SRE_TZ3_REGULAR", "", "2026-10-13"),
		},
	}
	r := NewTeamScheduleResolver(rota, sreTeams, nil)
	at := ist(2026, 10, 14, 2, 0)
	if got := ladderAt(t, r, "Artemis", at); got != [3]string{"ap-n1", "", ""} {
		t.Fatalf("Artemis incident at 02:00: L1/L2/L3 = %v, want [ap-n1 '' ''] -- regular hours are not a tier", got)
	}

	// And in the full week, where the same regular-hours row sits beside a
	// whole crew, it is never the one reached.
	week := nightCoverWeek(t)
	for _, team := range nightIncidents {
		for _, name := range ladderAt(t, week, team, at) {
			if name == "ar-reg" {
				t.Errorf("incident of team %q reached ar-reg, who is on TZ3 regular hours only", team)
			}
		}
	}
}
