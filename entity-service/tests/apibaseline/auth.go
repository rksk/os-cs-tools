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
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const keyID = "baseline-key"

func cmdKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	dir := fs.String("dir", "", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		return err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	if err := os.WriteFile(filepath.Join(*dir, "private.pem"), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		return err
	}
	jwks := map[string]any{"keys": []any{map[string]any{
		"kty": "RSA", "kid": keyID, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}}}
	b, _ := json.Marshal(jwks)
	return os.WriteFile(filepath.Join(*dir, "jwks.json"), b, 0o600)
}

func cmdJWKS(args []string) error {
	fs := flag.NewFlagSet("jwks", flag.ContinueOnError)
	dir := fs.String("dir", "", "key directory")
	port := fs.Int("port", 0, "listen port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(*dir, "jwks.json"))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})
	return http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", *port), mux)
}

type signer struct {
	key    *rsa.PrivateKey
	issuer string
}

func loadSigner(dir, issuer string) (*signer, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "private.pem"))
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(raw)
	if blk == nil {
		return nil, fmt.Errorf("no PEM block in private.pem")
	}
	k, err := x509.ParsePKCS1PrivateKey(blk.Bytes)
	if err != nil {
		return nil, err
	}
	return &signer{key: k, issuer: issuer}, nil
}

func (s *signer) sign(claims map[string]any) string {
	enc := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	head := enc(map[string]any{"alg": "RS256", "typ": "JWT", "kid": keyID})
	msg := head + "." + enc(claims)
	sum := sha256.Sum256([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	if err != nil {
		panic(err)
	}
	return msg + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// userToken mints the x-user-id-token the gateway would forward.
func (s *signer) userToken(email string, roles []string) string {
	now := time.Now()
	c := map[string]any{
		"iss": s.issuer, "aud": "baseline-spa", "sub": "sub-" + email,
		"email": email, "userid": "user-" + email,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}
	if len(roles) > 0 {
		c["roles"] = roles
	}
	return s.sign(c)
}

// clientAssertion mints the x-jwt-assertion; the server only decodes it.
func (s *signer) clientAssertion(clientID string) string {
	now := time.Now()
	return s.sign(map[string]any{
		"iss": s.issuer, "client_id": clientID,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
}

func cmdToken(args []string) error {
	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	dir := fs.String("dir", "", "key directory")
	issuer := fs.String("issuer", "https://issuer.example.invalid/token", "issuer")
	email := fs.String("email", "", "user email (omit for a client-only token)")
	client := fs.String("client", "", "client id for x-jwt-assertion")
	roles := fs.String("roles", "", "comma separated roles claim")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := loadSigner(*dir, *issuer)
	if err != nil {
		return err
	}
	if *client != "" {
		fmt.Printf("-H 'x-jwt-assertion: %s' ", s.clientAssertion(*client))
	}
	if *email != "" {
		var r []string
		if *roles != "" {
			r = strings.Split(*roles, ",")
		}
		fmt.Printf("-H 'x-user-id-token: %s'", s.userToken(*email, r))
	}
	fmt.Println()
	return nil
}
