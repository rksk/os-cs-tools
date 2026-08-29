// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// Licensed under the Apache License, Version 2.0 (the "License").
//
// Synthetic data seeder for the entity-service performance playground.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	mrand "math/rand"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type uuid [16]byte

func newUUID() uuid {
	var b uuid
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant
	return b
}

func atoiOr(s string, d int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return d
}

func pick[T any](r *mrand.Rand, s []T) T { return s[r.Intn(len(s))] }

// seed generates a consistent, realistic dataset sized by the -accounts / -projects /
// -deployments / -cases / -comments flags. It uses COPY for throughput and disables
// the user triggers on cases/case_comments during the load (the generated rows already
// satisfy the referential and CHECK constraints).
func seed(ctx context.Context, dsn string, opts map[string]string) {
	r := mrand.New(mrand.NewSource(42))

	nUsers := atoiOr(opts["users"], 500)
	nAccounts := atoiOr(opts["accounts"], 200)
	nProjects := atoiOr(opts["projects"], 1000)
	nDeployments := atoiOr(opts["deployments"], 2000)
	nCases := atoiOr(opts["cases"], 100000)
	commentsPer := atoiOr(opts["comments"], 5)

	conn := connect(ctx, dsn)
	defer conn.Close(ctx)

	exec := func(sql string) {
		if _, err := conn.Exec(ctx, sql); err != nil {
			log.Fatalf("exec %q: %v", sql, err)
		}
	}

	log.Printf("seeding: users=%d accounts=%d projects=%d deployments=%d cases=%d comments/case=%d",
		nUsers, nAccounts, nProjects, nDeployments, nCases, commentsPer)
	overall := time.Now()

	exec("ALTER TABLE cases DISABLE TRIGGER USER")
	exec("ALTER TABLE case_comments DISABLE TRIGGER USER")
	defer func() {
		exec("ALTER TABLE cases ENABLE TRIGGER USER")
		exec("ALTER TABLE case_comments ENABLE TRIGGER USER")
	}()

	copyIn := func(table string, cols []string, n int, row func(i int) []any) {
		start := time.Now()
		count, err := conn.CopyFrom(ctx, pgx.Identifier{table}, cols,
			pgx.CopyFromSlice(n, func(i int) ([]any, error) { return row(i), nil }))
		if err != nil {
			log.Fatalf("copy %s: %v", table, err)
		}
		log.Printf("  %-18s %8d rows (%s)", table, count, time.Since(start).Round(time.Millisecond))
	}

	now := time.Now().UTC()
	past := func(maxDays int) time.Time { return now.AddDate(0, 0, -r.Intn(maxDays)) }

	// ---- products + versions (small, fixed catalogue) ----
	type prod struct {
		id    uuid
		class string
	}
	prodDefs := []struct {
		name, class string
	}{
		{"API Manager", "software"}, {"Identity Server", "software"}, {"Micro Integrator", "software"},
		{"Enterprise Integrator", "software"}, {"Ballerina", "software"}, {"Streaming Integrator", "software"},
		{"Choreo", "service"}, {"Asgardeo", "service"}, {"Open Banking", "software"},
		{"API Platform for K8s", "software"}, {"Identity Server for K8s", "software"}, {"Container Service", "service"},
	}
	products := make([]prod, len(prodDefs))
	copyIn("products", []string{"id", "name", "class"}, len(prodDefs), func(i int) []any {
		id := newUUID()
		products[i] = prod{id: id, class: prodDefs[i].class}
		return []any{id, "WSO2 " + prodDefs[i].name, prodDefs[i].class}
	})

	type ver struct {
		id, productID uuid
	}
	var versions []ver
	verRows := [][]any{}
	statuses := []string{"available", "deprecated", "extended", "discontinued"}
	for _, p := range products {
		if p.class != "software" { // a trigger forbids versions for service-class products
			continue
		}
		for v := 1; v <= 3; v++ {
			id := newUUID()
			versions = append(versions, ver{id: id, productID: p.id})
			verRows = append(verRows, []any{
				id, p.id, fmt.Sprintf("%d.%d.0", v+2, r.Intn(9)),
				pick(r, statuses), past(2000), past(200),
			})
		}
	}
	copyIn("product_versions",
		[]string{"id", "product_id", "version", "current_support_status", "release_date", "support_eol_date"},
		len(verRows), func(i int) []any { return verRows[i] })
	// version index by product
	versByProduct := map[uuid][]uuid{}
	for _, v := range versions {
		versByProduct[v.productID] = append(versByProduct[v.productID], v.id)
	}

	// ---- users ----
	userIDs := make([]uuid, nUsers)
	var internalIDs []uuid
	firstNames := []string{"Alex", "Sam", "Jordan", "Taylor", "Riley", "Casey", "Morgan", "Jamie", "Devin", "Quinn"}
	lastNames := []string{"Silva", "Perera", "Fernando", "Smith", "Khan", "Tan", "Mendez", "Ono", "Ito", "Roy"}
	tzs := []string{"UTC", "Asia/Colombo", "America/New_York", "Europe/London", "Australia/Sydney"}
	copyIn("users",
		[]string{"id", "user_name", "first_name", "last_name", "email", "phone", "timezone", "user_type"},
		nUsers, func(i int) []any {
			id := newUUID()
			userIDs[i] = id
			internal := r.Float64() < 0.4
			ut := "customer"
			if internal {
				ut = "internal"
				internalIDs = append(internalIDs, id)
			}
			email := fmt.Sprintf("user%d@%s", i, map[bool]string{true: "wso2.com", false: "example.com"}[internal])
			return []any{id, email, pick(r, firstNames), pick(r, lastNames), email,
				fmt.Sprintf("+94%09d", r.Intn(1e9)), pick(r, tzs), ut}
		})
	if len(internalIDs) == 0 { // guarantee at least one internal user for owner/created_by FKs
		internalIDs = append(internalIDs, userIDs[0])
	}

	// ---- accounts ----
	accountIDs := make([]uuid, nAccounts)
	tiers := []string{"basic", "enterprise"}
	regions := []string{"NA", "EMEA", "APAC", "LATAM"}
	copyIn("accounts",
		[]string{"id", "sf_id", "name", "tier", "region", "activation_date", "owner_id", "technical_owner_id", "agent_enabled", "kb_references_enabled"},
		nAccounts, func(i int) []any {
			id := newUUID()
			accountIDs[i] = id
			return []any{id, fmt.Sprintf("SF-ACC-%06d", i), fmt.Sprintf("Account %d", i),
				pick(r, tiers), pick(r, regions), past(1500), pick(r, internalIDs), pick(r, internalIDs),
				r.Float64() < 0.5, r.Float64() < 0.5}
		})

	// ---- projects ----
	projectIDs := make([]uuid, nProjects)
	subs := []string{"development_support", "managed_cloud_subscription", "evaluation_subscription",
		"subscription", "cloud_support", "professional_services"}
	closures := []any{nil, "open", "notify", "read_only", "restricted"}
	copyIn("projects",
		[]string{"id", "account_id", "sf_id", "name", "key", "subscription_type", "closure_status", "start_date", "end_date"},
		nProjects, func(i int) []any {
			id := newUUID()
			projectIDs[i] = id
			start := past(1400)
			return []any{id, pick(r, accountIDs), fmt.Sprintf("SF-PRJ-%07d", i),
				fmt.Sprintf("Project %d", i), fmt.Sprintf("PRJ-%07d", i),
				pick(r, subs), pick(r, closures), start, start.AddDate(1, 0, 0)}
		})

	// ---- deployments (track project) ----
	deployIDs := make([]uuid, nDeployments)
	deployProject := make([]uuid, nDeployments)
	depTypes := []string{"primary_production", "staging", "qa", "stress", "uat", "development"}
	copyIn("deployments",
		[]string{"id", "project_id", "name", "type", "created_by"},
		nDeployments, func(i int) []any {
			id := newUUID()
			deployIDs[i] = id
			deployProject[i] = pick(r, projectIDs)
			return []any{id, deployProject[i], fmt.Sprintf("env-%07d", i), pick(r, depTypes), pick(r, internalIDs)}
		})

	// ---- deployed_products: one per deployment ----
	deployedProd := make([]uuid, nDeployments) // deployed_product id per deployment index
	copyIn("deployed_products",
		[]string{"id", "deployment_id", "product_id", "product_version_id"},
		nDeployments, func(i int) []any {
			id := newUUID()
			deployedProd[i] = id
			p := pick(r, products)
			vs := versByProduct[p.id]
			var versionID any // NULL for service-class products (no versions)
			if len(vs) > 0 {
				versionID = vs[r.Intn(len(vs))]
			}
			return []any{id, deployIDs[i], p.id, versionID}
		})

	// ---- cases ----
	caseIDs := make([]uuid, nCases)
	caseCreated := make([]time.Time, nCases)
	caseTypes := []string{"case", "case", "case", "case", "service_request", "engagement", "security_report_analysis", "announcement"}
	severities := []string{"critical", "high", "medium", "low"}
	issueTypes := []string{"error", "partial_outage", "performance_degradation", "question", "security_or_compliance", "total_outage"}
	engTypes := []string{"migration", "consultancy", "new_feature_improvement", "follow_up", "onboarding"}
	// states excluding work_in_progress (keeps work_state NULL and avoids the one-ongoing-per-engineer index)
	openStates := []string{"open", "waiting_on_wso2", "awaiting_info", "reopened", "solution_proposed"}
	copyIn("cases",
		[]string{"id", "created_by", "project_id", "deployment_id", "deployed_product_id",
			"type", "subject", "description", "severity", "issue_type", "state",
			"engagement_type", "created_at", "updated_at", "closed_at", "assigned_engineer", "work_state"},
		nCases, func(i int) []any {
			id := newUUID()
			caseIDs[i] = id
			di := r.Intn(nDeployments)
			typ := pick(r, caseTypes)
			var severity, engType, workState any
			if typ == "case" {
				severity = pick(r, severities)
			}
			if typ == "engagement" {
				engType = pick(r, engTypes)
			}
			created := past(730)
			caseCreated[i] = created
			// ~45% closed, else an open-ish state
			var state string
			var closedAt any
			if r.Float64() < 0.45 {
				state = "closed"
				closedAt = created.AddDate(0, 0, r.Intn(60)+1)
			} else if typ == "announcement" {
				state = "open"
			} else {
				state = pick(r, openStates)
			}
			var assignee any
			if r.Float64() < 0.8 {
				assignee = pick(r, internalIDs)
			}
			subj := fmt.Sprintf("[%s] %s on env – ticket %d", typ, pick(r, issueTypes), i)
			return []any{id, pick(r, internalIDs), deployProject[di], deployIDs[di], deployedProd[di],
				typ, subj, loremFor(r, i), severity, pick(r, issueTypes), state,
				engType, created, created, closedAt, assignee, workState}
		})

	// ---- case_comments ----
	type cmt struct {
		caseIdx int
		seq     int
	}
	totalComments := nCases * commentsPer
	commentTypes := []string{"comment", "comment", "comment", "work_note"}
	copyIn("case_comments",
		[]string{"case_id", "type", "content", "created_by", "created_at"},
		totalComments, func(i int) []any {
			c := cmt{caseIdx: i / commentsPer, seq: i % commentsPer}
			created := caseCreated[c.caseIdx].Add(time.Duration(c.seq+1) * time.Hour)
			return []any{caseIDs[c.caseIdx], pick(r, commentTypes),
				fmt.Sprintf("Update #%d: %s", c.seq+1, loremFor(r, i)),
				pick(r, userIDs), created}
		})

	log.Printf("ANALYZE…")
	exec("ANALYZE")
	log.Printf("seed complete in %s", time.Since(overall).Round(time.Millisecond))
}

var loremWords = []string{
	"customer", "deployment", "gateway", "token", "latency", "restart", "node", "cluster", "timeout",
	"certificate", "renewal", "throughput", "memory", "leak", "upgrade", "patch", "rollback", "incident",
	"investigation", "root", "cause", "mitigation", "workaround", "escalation", "priority", "reproduce",
}

func loremFor(r *mrand.Rand, salt int) string {
	n := 12 + r.Intn(28)
	out := make([]byte, 0, n*8)
	for i := 0; i < n; i++ {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, loremWords[(salt+i*7+r.Intn(len(loremWords)))%len(loremWords)]...)
	}
	return string(out)
}
