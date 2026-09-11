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

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/config"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/db"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/server"
)

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("load .env: %v", err)
	}

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	// A database is mandatory for DATA_SOURCE=postgres and optional for
	// servicenow, where entity traffic goes to the SN integration service
	// instead. With no database configured the two Postgres-only feature sets
	// (event_publish_failures, sla_clocks) are left unregistered rather than
	// failing startup — see config.Config.HasDatabase and server.NewRouter.
	var pool *pgxpool.Pool
	if cfg.HasDatabase() {
		var err error
		pool, err = db.NewPoolFromConfig(cfg)
		if err != nil {
			log.Fatalf("connect to database: %v", err)
		}
		defer pool.Close()
	} else {
		log.Printf("no database configured (DATA_SOURCE=%s): event-publish-failures and sla-clocks endpoints are disabled", cfg.DataSource)
	}

	addr := ":" + cfg.ServerPort
	srv, eventPublisher := server.New(addr, pool, cfg)

	go func() {
		log.Printf("Customer Entity REST Service started in PORT : %s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	if eventPublisher != nil {
		eventPublisher.Close()
	}
	log.Println("server stopped")
}
