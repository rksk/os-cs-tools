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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package main

import (
	"fmt"
	"time"
)

// Request-timeout defaults. These intentionally raise the previous values
// (server 30s, entity client 25s) so large inline-attachment uploads, which are
// relayed through two hops, are not cut off. They match the customer-portal
// backend: server 60s > entity client 55s, so the entity client always gives up
// first and the handler can still return a clean error.
const (
	defaultRESTReadTimeout      = 60 * time.Second
	defaultRESTWriteTimeout     = 60 * time.Second
	defaultEntityServiceTimeout = 55 * time.Second
)

// timeouts holds the operator-configurable request timeouts.
type timeouts struct {
	RESTRead      time.Duration
	RESTWrite     time.Duration
	EntityService time.Duration
}

// loadTimeouts resolves the timeouts from the environment via getenv. Each
// value is a Go duration string (e.g. "45s", "2m"); unset or empty selects the
// default. Every value must be > 0, and ENTITY_SERVICE_TIMEOUT must be strictly
// less than REST_WRITE_TIMEOUT so the upstream call times out before the
// response deadline does.
func loadTimeouts(getenv func(string) string) (timeouts, error) {
	var t timeouts
	for _, f := range []struct {
		key string
		def time.Duration
		dst *time.Duration
	}{
		{"REST_READ_TIMEOUT", defaultRESTReadTimeout, &t.RESTRead},
		{"REST_WRITE_TIMEOUT", defaultRESTWriteTimeout, &t.RESTWrite},
		{"ENTITY_SERVICE_TIMEOUT", defaultEntityServiceTimeout, &t.EntityService},
	} {
		d, err := durationEnv(getenv, f.key, f.def)
		if err != nil {
			return timeouts{}, err
		}
		*f.dst = d
	}
	if t.EntityService >= t.RESTWrite {
		return timeouts{}, fmt.Errorf(
			"ENTITY_SERVICE_TIMEOUT (%s) must be strictly less than REST_WRITE_TIMEOUT (%s)",
			t.EntityService, t.RESTWrite)
	}
	return t, nil
}

func durationEnv(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	raw := getenv(key)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q: %w", key, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s: duration must be > 0, got %q", key, raw)
	}
	return d, nil
}
