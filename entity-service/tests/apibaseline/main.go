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

// Command apibaseline is a black-box HTTP regression suite for the
// entity-service. It talks only HTTP to a running server and writes
// normalized results, so two builds of the service can be diffed.
//
// Subcommands (normally driven by run.sh):
//
//	keygen  --dir D                 write a throwaway token-signing key
//	jwks    --dir D --port P        serve the matching key set
//	token   --dir D --issuer I ...  mint a token (for manual curl exploration)
//	run     --base-url U ...        run every scenario, write results JSON
//	compare A.json B.json           per-step differences, exit 1 if any
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: apibaseline keygen|jwks|token|run|compare ...")
		os.Exit(2)
	}
	args := os.Args[2:]
	var err error
	code := 0
	switch os.Args[1] {
	case "keygen":
		err = cmdKeygen(args)
	case "jwks":
		err = cmdJWKS(args)
	case "token":
		err = cmdToken(args)
	case "run":
		code, err = cmdRun(args)
	case "compare":
		code, err = cmdCompare(args)
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "apibaseline:", err)
		os.Exit(2)
	}
	os.Exit(code)
}
