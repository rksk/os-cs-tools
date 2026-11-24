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

// seedNames maps the fixed ids in seed.sql to readable aliases used in results.
var seedNames = map[string]string{
	"00000000-0000-0000-0000-000000000001": "user:jane",
	"00000000-0000-0000-0000-000000000011": "user:alice",
	"00000000-0000-0000-0000-000000000012": "user:bob",
	"00000000-0000-0000-0000-000000000013": "user:carol",
	"00000000-0000-0000-0000-000000000031": "user:dormant",
	"00000000-0000-0000-0000-000000000021": "user:dave",
	"00000000-0000-0000-0000-000000000022": "user:erin",
	"00000000-0000-0000-0000-000000000003": "user:sam",
	"00000000-0000-0000-0000-000000000301": "account:example",
	"00000000-0000-0000-0000-000000000302": "account:other",
	"00000000-0000-0000-0000-000000000401": "project:example",
	"00000000-0000-0000-0000-000000000402": "project:other",
	"00000000-0000-0000-0000-000000000501": "deployment:prod",
	"00000000-0000-0000-0000-000000000502": "deployment:staging",
	"00000000-0000-0000-0000-000000000541": "deployment:other",
	"00000000-0000-0000-0000-000000000511": "product:gateway",
	"00000000-0000-0000-0000-000000000512": "product:identity",
	"00000000-0000-0000-0000-000000000521": "version:gw430",
	"00000000-0000-0000-0000-000000000522": "version:gw440",
	"00000000-0000-0000-0000-000000000523": "version:id700",
	"00000000-0000-0000-0000-000000000531": "dp:gw430",
	"00000000-0000-0000-0000-000000000532": "dp:id700",
	"00000000-0000-0000-0000-000000000533": "dp:gw440",
	"00000000-0000-0000-0000-000000000534": "dp:other",
	"00000000-0000-0000-0000-000000000901": "team:abt",
	"00000000-0000-0000-0000-000000000601": "case:seed1",
	"00000000-0000-0000-0000-000000000602": "case:seed2",
	"00000000-0000-0000-0000-000000000603": "case:seed3",
	"00000000-0000-0000-0000-000000000701": "comment:seed1",
	"00000000-0000-0000-0000-000000000801": "timecard:seed1",
	"00000000-0000-0000-0000-0000000000a3": "projecttype:sub",
}
