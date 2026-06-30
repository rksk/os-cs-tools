// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// Licensed under the Apache License, Version 2.0 (the "License").
//
// dbtool is a POC helper (entity-pg-poc branch) that bootstraps a local
// Postgres for the entity-service: it creates the database, applies the
// up-migrations in order, and seeds synthetic data for performance testing.
//
// Usage:
//
//	go run ./cmd/dbtool -dsn <maintenance-dsn> -db entity migrate
//	go run ./cmd/dbtool -dsn <entity-dsn> seed -accounts 500 -cases 200000
//	go run ./cmd/dbtool -dsn <maintenance-dsn> -db entity reset   # drop+recreate+migrate
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	log.SetFlags(0)
	args := os.Args[1:]
	// crude flag parsing: leading -flags then a command
	opts := map[string]string{
		"dsn":          envOr("DBTOOL_DSN", "postgres://postgres@/postgres?host=/tmp&port=5599"),
		"db":           "entity",
		"migrations":   "migrations",
		"accounts":     "200",
		"projects":     "1000",
		"users":        "500",
		"deployments":  "2000",
		"cases":        "100000",
		"comments":     "5",
	}
	var cmd string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			key := strings.TrimLeft(a, "-")
			if i+1 < len(args) {
				opts[key] = args[i+1]
				i++
			}
			continue
		}
		cmd = a
	}
	if cmd == "" {
		log.Fatal("usage: dbtool [flags] <migrate|seed|reset>")
	}

	ctx := context.Background()
	switch cmd {
	case "migrate":
		mustCreateDB(ctx, opts["dsn"], opts["db"])
		applyMigrations(ctx, dsnForDB(opts["dsn"], opts["db"]), opts["migrations"])
	case "reset":
		dropDB(ctx, opts["dsn"], opts["db"])
		mustCreateDB(ctx, opts["dsn"], opts["db"])
		applyMigrations(ctx, dsnForDB(opts["dsn"], opts["db"]), opts["migrations"])
	case "seed":
		seed(ctx, dsnForDB(opts["dsn"], opts["db"]), opts)
	default:
		log.Fatalf("unknown command %q", cmd)
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// dsnForDB swaps the database name in a key/value or URL DSN.
func dsnForDB(dsn, db string) string {
	// our default DSN is URL form: postgres://postgres@/postgres?host=/tmp&port=5599
	if strings.HasPrefix(dsn, "postgres://") {
		// replace the path component
		schemeAndRest := strings.SplitN(dsn, "://", 2)
		hostPart := schemeAndRest[1]
		q := ""
		if i := strings.Index(hostPart, "?"); i >= 0 {
			q = hostPart[i:]
			hostPart = hostPart[:i]
		}
		// hostPart = user@host/dbname  (host may be empty for socket)
		slash := strings.LastIndex(hostPart, "/")
		base := hostPart
		if slash >= 0 {
			base = hostPart[:slash]
		}
		return "postgres://" + base + "/" + db + q
	}
	return dsn // assume caller already pointed at the right db
}

func connect(ctx context.Context, dsn string) *pgx.Conn {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Fatalf("connect %s: %v", dsn, err)
	}
	return conn
}

func mustCreateDB(ctx context.Context, maintenanceDSN, db string) {
	conn := connect(ctx, maintenanceDSN)
	defer conn.Close(ctx)
	var exists bool
	_ = conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", db).Scan(&exists)
	if exists {
		log.Printf("database %q already exists", db)
		return
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q`, db)); err != nil {
		log.Fatalf("create database %q: %v", db, err)
	}
	log.Printf("created database %q", db)
}

func dropDB(ctx context.Context, maintenanceDSN, db string) {
	conn := connect(ctx, maintenanceDSN)
	defer conn.Close(ctx)
	_, _ = conn.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, db))
	log.Printf("dropped database %q", db)
}

func applyMigrations(ctx context.Context, dsn, dir string) {
	conn := connect(ctx, dsn)
	defer conn.Close(ctx)

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("read migrations dir %s: %v", dir, err)
	}
	var ups []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	for _, name := range ups {
		path := filepath.Join(dir, name)
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("read %s: %v", path, err)
		}
		start := time.Now()
		// PgConn().Exec uses the simple query protocol, which allows multiple
		// statements in one migration file.
		mrr := conn.PgConn().Exec(ctx, string(sqlBytes))
		if _, err := mrr.ReadAll(); err != nil {
			log.Fatalf("apply %s: %v", name, err)
		}
		log.Printf("applied %-45s (%s)", name, time.Since(start).Round(time.Millisecond))
	}
	log.Printf("migrations complete (%d files)", len(ups))
}

